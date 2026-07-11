'use strict';

// Dedicated matchmaker service — the processor of the shared Redis queue that every
// gateway feeds. It forms matches, allocates a fleet instance, and pushes each player's
// MatchmakingSussNtf to their owning gateway via gw.push (presence-routed).
//
// HA: run ONE OR MORE replicas. They elect a leader via a Redis lease (see
// tcp/matchmaker.js) — exactly one is the ACTIVE processor and the rest stand by, taking
// over automatically if it dies. No SPOF. Requires REDIS_URL; needs DATABASE_URL to build
// prepare_tokens with real cosmetics (else getById returns null).
const logger = require('../logger');
const config = require('../../config/default');
const matchmaker = require('../tcp/matchmaker');
const { getBus } = require('../bus/instance');

function main() {
  if (!getBus()) {
    logger.error('[matchmaker] REDIS_URL is required (the queue + leader lease live in Redis). Exiting.');
    process.exit(1);
  }
  if (!config.postgres.url) {
    logger.warn('[matchmaker] DATABASE_URL not set — prepare_tokens fall back to default cosmetics.');
  }
  matchmaker.startService();
  logger.info('[matchmaker] up (leader-elected; standing by until it holds the lease)');

  let shuttingDown = false;
  const shutdown = async (sig) => {
    if (shuttingDown) return;
    shuttingDown = true;
    logger.info(`[matchmaker] ${sig} — releasing lease + shutting down`);
    // Hand leadership over instantly (release the lease) so a standby doesn't idle out the TTL.
    try { await matchmaker.stopService(); } catch (_) { /* ignore */ }
    process.exit(0);
  };
  process.on('SIGTERM', () => shutdown('SIGTERM'));
  process.on('SIGINT', () => shutdown('SIGINT'));
}

main();
