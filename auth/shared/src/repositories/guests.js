'use strict';

// guests repository — backend dispatcher. Given the handle from store.connect(),
// returns the Postgres or Mongo implementation. Both expose an identical method
// surface, so callers (login-flow, routes/oauth) are backend-agnostic.
const { createGuestsRepoMongo } = require('./guests.mongo');
const { createGuestsRepoPg } = require('./guests.pg');

function createGuestsRepo(handle) {
  if (handle && handle.backend === 'pg') return createGuestsRepoPg(handle);
  // Back-compat: the Mongo path historically received the raw `db`. Accept either
  // a store handle ({ backend:'mongo', db }) or a bare db handle.
  const db = handle && handle.backend ? handle.db : handle;
  return createGuestsRepoMongo(db);
}

module.exports = { createGuestsRepo };
