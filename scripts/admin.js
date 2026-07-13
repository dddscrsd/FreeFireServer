#!/usr/bin/env node
'use strict';

// Admin console CLI over src/admin/core.js.
//   node scripts/admin.js                  -> interactive REPL (admin> ...)
//   node scripts/admin.js <cmd> [args...]  -> one-shot, prints result, exits
//   node scripts/admin.js --actor alice ... -> stamp the audit log with an actor
//
// Commands: whois | nick | role | kick | ban | unban | online | stats | find | help | quit
// It talks to the same Redis + Postgres the servers use (via config/.env), so run
// it on the internal network — NEVER expose it publicly.

require('dotenv').config();
const readline = require('readline');
const { AdminCore } = require('../src/admin/core');

const HELP = [
  'commands:',
  '  whois <id>              show a player (nickname, role, banned, online node)',
  '  nick  <id> <name...>    set nickname',
  '  role  <id> <n>          set role (integer)',
  '  kick  <id> [reason...]  TCP + match kick (drops lobby socket + ejects match)',
  '  ban   <id> [reason...]  set ban flags (game + auth) and kick',
  '  unban <id>              clear ban flags',
  '  online                  list online players (uid -> node)',
  '  stats                   online count, matchmaking queue, match fleet, redis/pg',
  '  find  <name...>         search players by nickname',
  '  help                    this text',
  '  quit                    exit (REPL only)',
].join('\n');

// Pull a leading "--actor NAME" out of argv (works for REPL launch + one-shot).
function extractActor(argv) {
  const i = argv.indexOf('--actor');
  if (i >= 0 && argv[i + 1]) { const actor = argv[i + 1]; argv.splice(i, 2); return actor; }
  return undefined;
}

function fmt(obj) { return JSON.stringify(obj, null, 2); }

function fmtWhois(w) {
  if (!w.found) return `not found${w.error ? ' (' + w.error + ')' : ''}${w.uid ? ' uid=' + w.uid : ''}`;
  return [
    `uid       ${w.uid}`,
    `nickname  ${w.nickname}`,
    `open_id   ${w.open_id || '-'}`,
    `role      ${w.role}`,
    `level     ${w.level}`,
    `banned    ${w.banned}${w.auth && w.auth.ban_reason ? ' (' + w.auth.ban_reason + ')' : ''}`,
    `online    ${w.online ? 'yes @ ' + w.node : 'no'}`,
  ].join('\n');
}

function fmtOnline(list) {
  if (!list.length) return '(nobody online)';
  return `${list.length} online:\n` + list.map((p) => `  ${p.uid}  @ ${p.node}`).join('\n');
}

function fmtStats(s) {
  return [
    `online         ${s.online}`,
    `mm queue       ${s.queue}`,
    `match servers  ${s.matchServers.length ? s.matchServers.join(', ') : '(none)'}`,
    `redis          ${s.redis ? 'up' : 'down'}`,
    `postgres       ${s.postgres ? 'up' : 'down'}`,
  ].join('\n');
}

function fmtFind(list) {
  if (!list.length) return '(no matches)';
  return list.map((p) => `  ${p.uid}  ${p.nickname}  (lvl ${p.level})`).join('\n');
}

// Dispatch one parsed command line. Returns a string to print.
async function dispatch(core, args) {
  const [verb, ...rest] = args;
  if (!verb) return '';
  const id = rest[0];
  switch (verb) {
    case 'whois': return fmtWhois(await core.whois(id));
    case 'nick':
    case 'nickname': return fmt(await core.setNickname(id, rest.slice(1).join(' ')));
    case 'role': return fmt(await core.setRole(id, rest[1]));
    case 'kick': return fmt(await core.kick(id, rest.slice(1).join(' ') || undefined));
    case 'ban': return fmt(await core.ban(id, rest.slice(1).join(' ') || undefined));
    case 'unban': return fmt(await core.unban(id));
    case 'online': return fmtOnline(await core.online());
    case 'stats': return fmtStats(await core.stats());
    case 'find': return fmtFind(await core.find(rest.join(' ')));
    case 'help': case '?': return HELP;
    default: return `unknown command: ${verb} (try 'help')`;
  }
}

async function runRepl(core) {
  const rl = readline.createInterface({ input: process.stdin, output: process.stdout, prompt: 'admin> ' });
  console.log("admin console — type 'help' for commands, 'quit' to exit");
  rl.prompt();
  rl.on('line', async (line) => {
    const args = line.trim().split(/\s+/).filter(Boolean);
    const verb = args[0];
    if (verb === 'quit' || verb === 'exit') { rl.close(); return; }
    if (verb) {
      try { console.log(await dispatch(core, args)); }
      catch (e) { console.error(`error: ${e.message}`); }
    }
    rl.prompt();
  });
  await new Promise((resolve) => rl.on('close', resolve));
}

async function main() {
  const argv = process.argv.slice(2);
  const actor = extractActor(argv);
  const core = new AdminCore({ actor });
  try {
    if (argv.length === 0) {
      await runRepl(core);
    } else {
      const out = await dispatch(core, argv);
      if (out) console.log(out);
    }
  } catch (e) {
    console.error(`error: ${e.stack || e.message}`);
    process.exitCode = 1;
  } finally {
    await core.close();
  }
}

main();
