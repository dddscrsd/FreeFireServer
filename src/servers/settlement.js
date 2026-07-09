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

const GROUP = 'settlement';

async function handleMatchResult(env, repo) {
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
  bus.subscribeStream('match.result', GROUP, consumer, (env) => handleMatchResult(env, repo));
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
