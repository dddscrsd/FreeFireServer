'use strict';

// pairings repository — backend dispatcher (see guests.js for the pattern).
const { createPairingsRepoMongo } = require('./pairings.mongo');
const { createPairingsRepoPg } = require('./pairings.pg');

function createPairingsRepo(handle) {
  if (handle && handle.backend === 'pg') return createPairingsRepoPg(handle);
  const db = handle && handle.backend ? handle.db : handle;
  return createPairingsRepoMongo(db);
}

module.exports = { createPairingsRepo };
