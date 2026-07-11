'use strict';

// Lazily-created, process-wide Bus singleton for the Node tier (gateway + HTTP
// handlers). Returns null when REDIS_URL is unset — the bus is OPTIONAL: presence
// and other cross-layer features degrade to no-ops while everything else keeps
// working. Callers must treat the result as best-effort (null-check + catch).
const config = require('../../config/default');
const { Bus } = require('./index');
const logger = require('../logger');

let created = false;
let bus = null;

function getBus() {
  if (created) return bus;
  created = true;
  const url = config.redis && config.redis.url;
  if (!url) return (bus = null);
  try {
    bus = new Bus({ url, source: process.env.BUS_SOURCE || config.nodeId || 'node', node: config.nodeId });
    logger.info('[bus] shared instance connected');
  } catch (err) {
    logger.error(`[bus] shared instance init failed: ${err.message}`);
    bus = null;
  }
  return bus;
}

module.exports = { getBus };
