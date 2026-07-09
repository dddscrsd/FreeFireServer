'use strict';

// The account repository: one async interface, two backends. Postgres when
// DATABASE_URL (config.postgres.url) is set, otherwise the existing SQLite store.
// Phase 1 keeps SQLite as the default; flipping DATABASE_URL is the cutover lever.
//
// Interface:
//   getByToken(token)      -> account | null
//   getById(uid)           -> account | null
//   getByOpenId(openId)    -> account | null
//   getByIds([uid])        -> account[]           (batch; kills the N+1 fan-outs)
//   searchByNickname(str)  -> account_id[]        (max 20)
//   createFromLogin(req)   -> account
//   save(account)          -> account
//   close()                -> Promise<void>
const config = require('../../../config/default');
const logger = require('../../logger');

let repo = null;

function getRepo() {
  if (repo) return repo;
  const url = config.postgres && config.postgres.url;
  if (url) {
    const { PostgresRepo } = require('./postgres');
    repo = new PostgresRepo(url);
    logger.info('[repo] backend: PostgreSQL');
  } else {
    repo = require('./sqlite');
    logger.info('[repo] backend: SQLite');
  }
  return repo;
}

module.exports = { getRepo };
