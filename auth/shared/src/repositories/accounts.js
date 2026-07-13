'use strict';

// accounts repository — backend dispatcher (see guests.js for the pattern).
const { createAccountsRepoMongo } = require('./accounts.mongo');
const { createAccountsRepoPg } = require('./accounts.pg');

function createAccountsRepo(handle) {
  if (handle && handle.backend === 'pg') return createAccountsRepoPg(handle);
  const db = handle && handle.backend ? handle.db : handle;
  return createAccountsRepoMongo(db);
}

module.exports = { createAccountsRepo };
