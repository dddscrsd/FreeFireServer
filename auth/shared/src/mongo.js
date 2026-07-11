'use strict';

const { MongoClient } = require('mongodb');
const config = require('./config');
const { logger } = require('./logger');

let client = null;

async function connect() {
  if (client) {
    return { client, db: client.db(config.DB_NAME) };
  }
  client = new MongoClient(config.MONGO_URI, {
    // Reasonable defaults; can be surfaced as env vars later if needed.
    maxPoolSize: 50,
    minPoolSize: 5,
    serverSelectionTimeoutMS: 5000,
    connectTimeoutMS: 5000,
    socketTimeoutMS: 15000,
  });
  try {
    await client.connect();
  } catch (err) {
    logger.error({ err }, 'mongo connection failed');
    throw err;
  }
  return { client, db: client.db(config.DB_NAME) };
}

async function ensureIndexes(db) {
  await db.collection('guests').createIndexes([
    { key: { open_id: 1 }, unique: true, background: true, name: 'open_id_unique' },
    { key: { dc_id: 1 }, unique: true, background: true, name: 'dc_id_unique' },
    { key: { access_token: 1 }, background: true, name: 'access_token' },
    { key: { refresh_token: 1 }, background: true, name: 'refresh_token' },
    { key: { uid: 1 }, background: true, name: 'uid' },
  ]);
  await db.collection('accounts').createIndexes([
    { key: { open_id: 1 }, unique: true, background: true, name: 'open_id_unique' },
    { key: { uid: 1 }, unique: true, background: true, name: 'uid_unique' },
  ]);
  // `pairings` documents are short-lived. The TTL index on `expires_at`
  // causes MongoDB's TTL monitor (runs ~every 60s) to remove abandoned
  // rows automatically, so we never accumulate stale bridges.
  await db.collection('pairings').createIndexes([
    { key: { pair_id: 1 }, unique: true, background: true, name: 'pair_id_unique' },
    { key: { expires_at: 1 }, expireAfterSeconds: 0, background: true, name: 'expires_at_ttl' },
  ]);
  logger.info('mongo indexes ensured');
}

async function close() {
  if (client) {
    try {
      await client.close();
    } finally {
      client = null;
    }
  }
}

module.exports = { connect, ensureIndexes, close };
