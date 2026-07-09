'use strict';

/**
 * Stub gateway matchmaker.
 *
 * Players join a queue on MATCHMAKING/START and leave on CANCEL (or disconnect).
 * A per-second tick forms a match for a mode-group once it reaches MIN_PLAYERS,
 * or when a queued player has waited TIMEOUT_MS; the matched players are each
 * sent a MatchmakingSussNtf pointing at the game server.
 *
 * Intentionally minimal: no skill/region/party matching and no real game-server
 * allocation — server_addr just points at the configured game-server host:port.
 * Tune with MM_MIN_PLAYERS / MM_TIMEOUT_MS.
 */

const logger = require('../logger');
const config = require('../../config/default');
const jwt = require('../utils/jwt');
const { getLocalIp } = require('../utils/address');
const { EProtocol, EMatchmaking } = require('./protocol');
const fleet = require('./fleet');

const MIN_PLAYERS = Number(process.env.MM_MIN_PLAYERS || 2);    // form a match at N players...
const TIMEOUT_MS = Number(process.env.MM_TIMEOUT_MS || 10000);  // ...or after this wait
const TICK_MS = 1;

// Resolve the LAN host once as a FALLBACK for the game-server address handed to
// clients; MATCH_PUBLIC_HOST (config.domains.match) overrides it in production so
// the client never receives the container's internal IP.
let host = '127.0.0.1';
getLocalIp().then((ip) => { if (ip) host = ip; }).catch(() => {});

let queue = [];   // [{ accountId, account, gateway, mode, joinedAt }]
let timer = null;
let matchSeq = 0;

const modeKey = (m) => `${m.match_mode}:${m.game_mode}:${m.map_id}:${m.difficulty || 0}`;

function ensureTimer() {
  if (!timer) {
    timer = setInterval(tick, TICK_MS);
    if (timer.unref) timer.unref();
  }
}
function stopTimer() {
  if (timer) { clearInterval(timer); timer = null; }
}

function enqueue(account, gateway, mode) {
  const accountId = account.uid || account.account_id;
  cancel(accountId, true); // de-dupe an existing entry
  queue.push({ accountId, account, gateway, mode, joinedAt: Date.now() });
  ensureTimer();
  logger.info(`[mm] enqueue uid=${accountId} mode=${modeKey(mode)} (queue=${queue.length})`);
}

function cancel(accountId, silent) {
  const before = queue.length;
  queue = queue.filter((e) => e.accountId !== accountId);
  if (queue.length !== before && !silent) {
    logger.info(`[mm] cancel uid=${accountId} (queue=${queue.length})`);
  }
  if (!queue.length) stopTimer();
}

function tick() {
  if (!queue.length) { stopTimer(); return; }
  const now = Date.now();
  const groups = new Map();
  for (const e of queue) {
    const k = modeKey(e.mode);
    if (!groups.has(k)) groups.set(k, []);
    groups.get(k).push(e);
  }
  for (const entries of groups.values()) {
    const enough = entries.length >= MIN_PLAYERS;
    const timedOut = entries.some((e) => now - e.joinedAt >= TIMEOUT_MS);
    if (enough || timedOut) formMatch(entries);
  }
}

// Build the prepare_token: an HS256 JWT carrying the player's identity and
// selected cosmetics so the (separate-process) Go match-server can build cmd 101
// PLAYER_JOIN from it without sharing our account DB. Verified there with the
// same MATCH_JWT_SECRET (see match-server/token/jwt.go).
function buildPrepareToken(account, matchId) {
  const acc = account || {};
  const sel = acc.selected_items || {};
  const claims = {
    aid: acc.uid || acc.account_id || 0,
    name: acc.nickname || 'Player',
    region: acc.region || '',
    role: acc.role || 0,
    mid: matchId,
    show: {
      avatar: sel.avatar_id || 0,
      color: sel.skin_color || 0,
      head: sel.head_pic || 0,
      banner: sel.banner_id || 0,
      clothes: Array.isArray(sel.clothes) ? sel.clothes : [],
      slots: Array.isArray(sel.slots) ? sel.slots : [],
      emotes: sel.emotes && Array.isArray(sel.emotes.emotes)
      ? sel.emotes.emotes.map(e => e.emote_id)
      : []
    }
  };
  const token = jwt.sign(claims, config.match.jwtSecret, { expiresInSec: config.match.jwtTtlSec });
  logger.info(`[mm] build_prepare_token aid=${account.uid || account.account_id} out=${token}`);
  return token;
}

function formMatch(entries) {
  const matchId = ++matchSeq;
  const mode = entries[0].mode;
  const matchHost = (config.domains && config.domains.match) || host;
  const staticAddr = `${matchHost}:${(config.protocol && config.protocol.gameServerPort) || '10100'}`;
  // Allocate this match to a LIVE instance from the fleet registry (all players in ONE
  // match get the SAME instance); fall back to the static address (single instance / no bus).
  const serverAddr = fleet.pickServer() || staticAddr;
  const secret = '00112233445566778899aabbccddeeff'; // hex; client does HexStringToByte()
  logger.info(
    `[mm] MATCH #${matchId} mode=${modeKey(mode)} players=[${entries.map((e) => e.accountId).join(',')}] -> ${serverAddr}`
  );
  for (const e of entries) {
    cancel(e.accountId, true);
    const prepareToken = buildPrepareToken(e.account, matchId);
    e.gateway.notify(
      e.accountId,
      EProtocol.MATCHMAKING,
      EMatchmaking.MATCHMAKINGSUSS_NTF,
      'MatchmakingSussNtf',
      {
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
      },
      0
    );
  }
}

module.exports = { enqueue, cancel, queueSize: () => queue.length };
