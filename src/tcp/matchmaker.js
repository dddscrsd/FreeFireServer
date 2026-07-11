'use strict';

/**
 * Shared matchmaker (HA / leader-elected).
 *
 * The queue lives in REDIS (hash `mmqueue`: accountId -> {node, mode, joinedAt}) so
 * EVERY gateway instance feeds ONE queue. The gateway process only enqueues/cancels
 * here (no local tick); a dedicated matchmaker service (src/servers/matchmaker.js)
 * calls startService() to run the tick: it forms matches, allocates a fleet instance
 * (see fleet.js), and pushes each player's MatchmakingSussNtf to their OWNING gateway
 * via gw.push (routed by presence).
 *
 * HA: you may run MULTIPLE matchmaker replicas. They contend for a Redis lease
 * (`mm:leader`); exactly ONE is the ACTIVE processor at a time and the rest stand by.
 * If the active one dies, the lease expires (MM_LEASE_MS) and a standby takes over —
 * no split-brain (the tick is gated on holding the lease) and no SPOF. A slow tick can
 * outlive the lease, so a formed match still claims its players atomically via HDEL,
 * which the takeover leader also respects (claimed players are already off the queue).
 *
 * Tune with MM_MIN_PLAYERS / MM_TIMEOUT_MS / MM_TICK_MS / MM_MATCH_SIZE / MM_LEASE_MS.
 */

const crypto = require('crypto');
const logger = require('../logger');
const config = require('../../config/default');
const jwt = require('../utils/jwt');
const { getLocalIp } = require('../utils/address');
const { EProtocol, EMatchmaking } = require('./protocol');
const { lookup } = require('../protocol/protos');
const fleet = require('./fleet');
const { getBus } = require('../bus/instance');
const { getRepo } = require('../db/repo');

const MIN_PLAYERS = Number(process.env.MM_MIN_PLAYERS || 2);    // form a match at N players...
const TIMEOUT_MS = Number(process.env.MM_TIMEOUT_MS || 10000);  // ...or after this wait
const TICK_MS = Number(process.env.MM_TICK_MS || 1000);         // service poll period (Redis-backed queue)
const MATCH_SIZE = Number(process.env.MM_MATCH_SIZE || 50);     // cap players per formed match (floods)
const LEASE_MS = Number(process.env.MM_LEASE_MS || 15000);      // leader lease TTL (failover time upper-bound)
const RENEW_MS = Math.max(1000, Math.floor(LEASE_MS / 3));      // renew the lease well within its TTL
const QUEUE_KEY = 'mmqueue';   // Redis hash: accountId -> JSON { node, mode, joinedAt }
const SEQ_KEY = 'mm:matchseq'; // Redis INCR: globally-unique, monotonic, restart-safe match id
const LOCK_KEY = 'mm:leader';  // Redis lease: the ACTIVE matchmaker holds this (leader election)

// LAN fallback for the game-server address handed to clients (fleet / MATCH_PUBLIC_HOST
// override it in production so the client never receives a container-internal IP).
let host = '127.0.0.1';
getLocalIp().then((ip) => { if (ip) host = ip; }).catch(() => {});

const modeKey = (m) => `${m.match_mode}:${m.game_mode}:${m.map_id}:${m.difficulty || 0}`;

function staticAddr() {
  const matchHost = (config.domains && config.domains.match) || host;
  return `${matchHost}:${(config.protocol && config.protocol.gameServerPort) || '10100'}`;
}

// --- gateway-facing API: write the SHARED queue (no local tick) -------------

function enqueue(account, _gateway, mode) {
  const accountId = account.uid || account.account_id;
  const bus = getBus();
  if (!bus || !accountId) return;
  bus.hset(QUEUE_KEY, accountId, JSON.stringify({ node: config.nodeId, mode, joinedAt: Date.now() }))
    .catch((e) => logger.warn(`[mm] enqueue uid=${accountId}: ${e.message}`));
  logger.info(`[mm] enqueue uid=${accountId} mode=${modeKey(mode)}`);
}

function cancel(accountId) {
  const bus = getBus();
  if (!bus || !accountId) return;
  bus.hdel(QUEUE_KEY, accountId).catch((e) => logger.warn(`[mm] cancel uid=${accountId}: ${e.message}`));
}

async function queueSize() {
  const bus = getBus();
  return bus ? bus.hlen(QUEUE_KEY) : 0;
}

// Build the prepare_token: an HS256 JWT carrying the player's identity and selected
// cosmetics so the (separate-process) Go match-server can build cmd 101 PLAYER_JOIN
// without sharing our account DB. Verified there with the same MATCH_JWT_SECRET.
function buildPrepareToken(account, matchId, team = 0) {
  const acc = account || {};
  const sel = acc.selected_items || {};
  const claims = {
    aid: acc.uid || acc.account_id || 0,
    name: acc.nickname || 'Player',
    region: acc.region || '',
    role: acc.role || 0,
    mid: matchId,
    // team: the custom-room team the host arranged (group_id 1 or 2). 0 = "server chooses" — the
    // matchmaker/queue path passes nothing, so the match server keeps its balance-fill. A room passes
    // the member's group_id so the match faction (entity-id hibyte + FACTION_ID) is deterministic.
    team,
    show: {
      avatar: sel.avatar_id || 0,
      color: sel.skin_color || 0,
      head: sel.head_pic || 0,
      banner: sel.banner_id || 0,
      clothes: Array.isArray(sel.clothes) ? sel.clothes : [],
      slots: Array.isArray(sel.slots) ? sel.slots : [],
      emotes: sel.emotes && Array.isArray(sel.emotes.emotes)
        ? sel.emotes.emotes.map((e) => e.emote_id)
        : []
    }
  };
  const token = jwt.sign(claims, config.match.jwtSecret, { expiresInSec: config.match.jwtTtlSec });
  logger.info(`[mm] build_prepare_token aid=${claims.aid} mid=${matchId}`);
  return token;
}

// --- matchmaker SERVICE: leader-elected processor of the shared queue --------

let timer = null;       // form-loop timer (runs only while we hold the lease)
let leaseTimer = null;  // leadership loop (acquire/renew the lease)
let ticking = false;
let isLeader = false;
let leaseToken = null;  // this replica's unique lease token (proves ownership on renew/release)

function startService() {
  if (timer) return;
  // Unique per PROCESS so two replicas can never collide on the same token.
  leaseToken = `${config.nodeId}#${process.pid}#${crypto.randomBytes(6).toString('hex')}`;
  leaseTimer = setInterval(() => { leadership().catch((e) => logger.error(`[mm] leadership: ${e.message}`)); }, RENEW_MS);
  leadership().catch(() => {}); // try to become leader immediately (a lone replica shouldn't wait a full period)
  timer = setInterval(() => { tick().catch((e) => logger.error(`[mm] tick: ${e.message}`)); }, TICK_MS);
  logger.info(`[mm] service up (tick=${TICK_MS}ms lease=${LEASE_MS}ms min=${MIN_PLAYERS} timeout=${TIMEOUT_MS}ms) token=${leaseToken}`);
}

// stopService relinquishes the lease (so a standby takes over INSTANTLY on graceful
// shutdown instead of waiting out the TTL) and stops the loops. Best-effort.
async function stopService() {
  if (leaseTimer) { clearInterval(leaseTimer); leaseTimer = null; }
  if (timer) { clearInterval(timer); timer = null; }
  const bus = getBus();
  if (isLeader && bus && leaseToken) {
    try { await bus.releaseLock(LOCK_KEY, leaseToken); logger.info('[mm] released leadership'); } catch (_) { /* ignore */ }
  }
  isLeader = false;
}

// leadership acquires (as a standby) or renews (as the leader) the Redis lease. Losing a
// renew (missed heartbeat / clock stall past the TTL) demotes us to standby, so at most one
// replica ever forms matches.
async function leadership() {
  const bus = getBus();
  if (!bus) return;
  if (isLeader) {
    if (await bus.renewLock(LOCK_KEY, leaseToken, LEASE_MS)) return;
    isLeader = false;
    logger.warn('[mm] LOST leadership (lease not renewed) — standby');
  }
  if (await bus.acquireLock(LOCK_KEY, leaseToken, LEASE_MS)) {
    isLeader = true;
    logger.info('[mm] ACQUIRED leadership — this replica is now the active matchmaker');
  }
}

async function tick() {
  if (!isLeader) return; // only the lease holder forms matches (no split-brain)
  if (ticking) return; // never overlap — a slow tick must not let the next re-form the same players
  const bus = getBus();
  if (!bus) return;
  ticking = true;
  try {
    const raw = await bus.hgetall(QUEUE_KEY);
    const ids = Object.keys(raw || {});
    if (!ids.length) return;
    const now = Date.now();
    const groups = new Map();
    for (const id of ids) {
      let e;
      try { e = JSON.parse(raw[id]); } catch (_) { continue; }
      e.accountId = Number(id);
      const k = modeKey(e.mode);
      if (!groups.has(k)) groups.set(k, []);
      groups.get(k).push(e);
    }
    for (const entries of groups.values()) {
      entries.sort((a, b) => a.joinedAt - b.joinedAt); // oldest first (fairness)
      const enough = entries.length >= MIN_PLAYERS;
      const timedOut = entries.some((e) => now - e.joinedAt >= TIMEOUT_MS);
      if (enough || timedOut) await formMatch(entries.slice(0, MATCH_SIZE));
    }
  } finally {
    ticking = false;
  }
}

async function formMatch(entries) {
  const bus = getBus();
  const matchId = await bus.incr(SEQ_KEY);
  const mode = entries[0].mode;
  const serverAddr = fleet.pickServer() || staticAddr();
  const secret = '00112233445566778899aabbccddeeff'; // hex; client does HexStringToByte()
  const ids = entries.map((e) => e.accountId);
  await bus.hdel(QUEUE_KEY, ...ids); // claim: remove from the shared queue so no other tick re-forms
  logger.info(`[mm] MATCH #${matchId} mode=${modeKey(mode)} players=[${ids.join(',')}] -> ${serverAddr}`);

  const Suss = lookup('MatchmakingSussNtf');
  for (const e of entries) {
    // Skip a player who disconnected while queued (presence gone) — they were already
    // removed from the queue above; nothing to push.
    if (!(await bus.getNode(e.accountId))) { logger.info(`[mm] skip offline uid=${e.accountId}`); continue; }
    const account = await getRepo().getById(e.accountId);
    if (!account) { logger.warn(`[mm] uid=${e.accountId} not found — skip`); continue; }
    const prepareToken = buildPrepareToken(account, matchId);
    const content = Suss ? Buffer.from(Suss.encode(Suss.fromObject({
      match_id: matchId,
      server_addr: serverAddr,
      secret,
      prepare_token: prepareToken,
      sleep_ms: 1,
      map_id: mode.map_id,
      game_mode: mode.game_mode,
      match_mode: 6 /*mode.match_mode*/, // forced 6 so client renders waiting for players dialogue
      difficulty: mode.difficulty || 0,
      use_cache: false,
      first_login: false
    })).finish()) : Buffer.alloc(0);
    // Push the SussNtf to whichever gateway holds this player (gw.push, presence-routed).
    await bus.publishPS('gw.push', 'GatewayPush', {
      target_account_id: e.accountId,
      protocol: EProtocol.MATCHMAKING,
      cmd: EMatchmaking.MATCHMAKINGSUSS_NTF,
      content
    });
  }
}

// nextMatchId mints a globally-unique, restart-safe match id (shared counter with the
// matchmaker, so room matches and queued matches never collide).
async function nextMatchId() { const bus = getBus(); return bus ? bus.incr(SEQ_KEY) : 0; }

// buildPrepareToken / staticAddr / nextMatchId are exported so the custom-room START handoff
// (src/tcp/handlers/RoomStart.js) reuses the SAME match machinery (token + fleet addr + id) the
// matchmaker uses — it just pushes the SussNtf under the ROOM protocol to the room's members
// instead of pulling from the shared queue.
module.exports = { enqueue, cancel, queueSize, startService, stopService, buildPrepareToken, staticAddr, nextMatchId };
