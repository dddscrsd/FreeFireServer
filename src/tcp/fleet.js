'use strict';

// Match-server fleet cache. Match instances self-register in Redis (see the Go
// match-server's fleetHeartbeat); this keeps a locally-cached list refreshed off the
// hot path so the matchmaker can pick a target instance SYNCHRONOUSLY when a match
// forms. An empty list => the matchmaker uses its static configured address (single
// instance / no bus), so this is fully backward compatible.
const { getBus } = require('../bus/instance');

const REFRESH_MS = 5000;
let servers = [];
let timer = null;

async function refresh() {
  try {
    const bus = getBus();
    if (bus) servers = await bus.getMatchServers();
  } catch (e) { /* keep the last good list on a Redis blip */ }
}

function start() {
  if (timer) return;
  refresh();
  timer = setInterval(refresh, REFRESH_MS);
  if (timer.unref) timer.unref();
}

// pickServer returns a registered match-server address (random spread across the
// fleet), or '' when none are registered. All players in a single match must be given
// the SAME address, so callers pick ONCE per match, not per player.
function pickServer() {
  if (!servers.length) return '';
  return servers[Math.floor(Math.random() * servers.length)];
}

start();

module.exports = { start, pickServer, list: () => servers.slice() };
