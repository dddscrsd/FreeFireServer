'use strict';

// Storage selector. Picks the PostgreSQL backend when DATABASE_URL is set,
// otherwise the original MongoDB backend — exactly like the game server's
// src/db/repo (SQLite/Postgres). Returns an opaque `handle` that the repository
// factories (repositories/*.js) dispatch on, so every route/flow file stays
// backend-agnostic and unchanged.
//
//   const handle = await store.connect();      // { backend, db | pool, schema }
//   await store.ensureSchema(handle);          // indexes (mongo) / DDL (pg)
//   const guests = createGuestsRepo(handle);   // dispatches on handle.backend
//   await store.close();

const config = require('./config');
const mongo = require('./mongo');
const pg = require('./pg');

async function connect() {
  if (config.DB_BACKEND === 'pg') {
    const { pool, schema } = await pg.connect();
    return { backend: 'pg', pool, schema };
  }
  const { db } = await mongo.connect();
  return { backend: 'mongo', db };
}

// Ensure indexes (Mongo) / schema + tables + indexes (Postgres). Idempotent.
async function ensureSchema(handle) {
  if (handle && handle.backend === 'pg') return pg.ensureSchema();
  return mongo.ensureIndexes(handle.db);
}

async function close() {
  if (config.DB_BACKEND === 'pg') return pg.close();
  return mongo.close();
}

module.exports = { connect, ensureSchema, close, backend: config.DB_BACKEND };
