'use strict';

// Settlement worker — consumes the durable `match.result` Stream the Go match server
// publishes at match end and persists each player's result to Postgres. This is the
// path (Phase 3) that stops match rewards/stats from evaporating: the Go server has no
// DB, so it emits results on the bus and this worker records + credits them,
// idempotently — at-least-once Stream delivery can't double-credit (see the ledger PK).
// Requires DATABASE_URL (it writes the match_results ledger and credits accounts).
const config = require('../../config/default');
const logger = require('../logger');
const { Bus } = require('../bus');
const { getRepo } = require('../db/repo');
const { EProtocol, EStats } = require('../tcp/protocol');
const { buildTeammates, buildStatsRes } = require('../tcp/matchstats');

const GROUP = 'settlement';

async function handleMatchResult(env, repo, bus) {
  const mr = Bus.payload(env, 'MatchResult');
  if (!mr.match_id || !Array.isArray(mr.players)) {
    logger.warn('[settlement] match.result missing match_id/players — dropping');
    return;
  }
  let credited = 0;
  for (const pr of mr.players) {
    if (!pr.account_id) continue; // no account (shouldn't happen) — skip
    const r = await repo.settleMatchResult(mr.match_id, pr);
    if (r.credited) credited++;
  }
  logger.info(`[settlement] match=${mr.match_id} players=${mr.players.length} credited=${credited}`);
  // Post-match lobby scoreboard: push MATCHSTATS_NTF to each ONLINE player with their
  // rewards (MatchIncome) + the full scoreboard (MatchStats.teammates). Best-effort —
  // the settled ledger is the source of truth; this is just the UI push.
  try { await pushMatchStats(mr, repo, bus); } catch (e) { logger.warn(`[settlement] match-stats push: ${e.message}`); }
}

// Push the post-match result UI to each connected player (protocol STATS /
// MATCHSTATS_NTF) via the gw.push relay, built from the same settled data.
async function pushMatchStats(mr, repo, bus) {
  const accts = {};
  await Promise.all(mr.players.map(async (pr) => {
    accts[pr.account_id] = await repo.getById(pr.account_id).catch(() => null);
  }));
  const teammates = buildTeammates(mr.players, accts);
  let pushed = 0;
  for (const pr of mr.players) {
    if (!(await bus.getNode(pr.account_id))) continue; // offline -> nothing to show
    const content = buildStatsRes(pr, teammates, accts[pr.account_id], mr.match_id, mr.players.length);
    if (!content) continue;
    await bus.publishPS('gw.push', 'GatewayPush', {
      target_account_id: pr.account_id,
      protocol: EProtocol.STATS,
      cmd: EStats.MATCHSTATS_NTF,
      content
    });
    pushed++;
  }
  if (pushed) logger.info(`[settlement] pushed match-stats to ${pushed} online player(s) for match=${mr.match_id}`);
}

async function main() {
  if (!config.postgres.url) {
    logger.error('[settlement] DATABASE_URL is required — settlement writes the Postgres ledger. Exiting.');
    process.exit(1);
  }
  const repo = getRepo();
  const bus = new Bus({ url: config.redis.url, source: 'settlement', node: config.nodeId });
  const consumer = config.nodeId;
  // A handler throw (e.g. a DB blip) leaves the message PENDING for redelivery — safe
  // because settleMatchResult is idempotent on (match_id, account_id).
  bus.subscribeStream('match.result', GROUP, consumer, (env) => handleMatchResult(env, repo, bus));
  logger.info(`[settlement] up — consuming stream:match.result as ${GROUP}/${consumer}`);

  const shutdown = async (sig) => {
    logger.info(`[settlement] ${sig} — shutting down`);
    try { await bus.close(); } catch (_) { /* ignore */ }
    try { if (repo.close) await repo.close(); } catch (_) { /* ignore */ }
    process.exit(0);
  };
  process.on('SIGTERM', () => shutdown('SIGTERM'));
  process.on('SIGINT', () => shutdown('SIGINT'));
}

main().catch((err) => { logger.error(`[settlement] fatal: ${err.message}`); process.exit(1); });
