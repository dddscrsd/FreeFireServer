'use strict';

// Dedicated matchmaker service — the SINGLE processor of the shared Redis queue that
// every gateway feeds. It forms matches, allocates a fleet instance, and pushes each
// player's MatchmakingSussNtf to their owning gateway via gw.push (presence-routed).
// Run exactly ONE instance (it reads/claims the whole queue). Requires REDIS_URL; needs
// DATABASE_URL to build prepare_tokens with real cosmetics (else getById returns null).
const logger = require('../logger');
const config = require('../../config/default');
const matchmaker = require('../tcp/matchmaker');
const { getBus } = require('../bus/instance');

function main() {
  if (!getBus()) {
    logger.error('[matchmaker] REDIS_URL is required (the queue lives in Redis). Exiting.');
    process.exit(1);
  }
  if (!config.postgres.url) {
    logger.warn('[matchmaker] DATABASE_URL not set — prepare_tokens fall back to default cosmetics.');
  }
  matchmaker.startService();
  logger.info('[matchmaker] up');

  const shutdown = (sig) => { logger.info(`[matchmaker] ${sig} — shutting down`); process.exit(0); };
  process.on('SIGTERM', () => shutdown('SIGTERM'));
  process.on('SIGINT', () => shutdown('SIGINT'));
}

main();
