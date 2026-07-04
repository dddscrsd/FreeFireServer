/**
 * _stubs.js — single generic stub for every endpoint without a real handler.
 *
 * Every endpoint listed in protocol/endpoint_map.json that does NOT already have
 * a dedicated handler module gets ONE shared `genericStub` registered against it.
 * The stub returns an empty object; the router then default-constructs the
 * endpoint's resType and sends it. This replaces the old generated_*.js stub
 * chunks (which each returned {} per endpoint) with a single function.
 *
 * Files beginning with '_' (this one, _shared.js) are NOT endpoint modules and
 * are skipped by the router's loader; this module is invoked explicitly, LAST,
 * by router.loadHandlers() so real handlers always win.
 */

'use strict';

const ENDPOINT_MAP = require('../../protocol/endpoint_map.json');

// One shared stub for all unhandled endpoints. Returns {}; the router encodes
// the endpoint's resType default-constructed (or 'Empty' when unknown).
function genericStub(reqObj, ctx) {
  return {};
}

/**
 * Register the generic stub for every endpoint in endpoint_map.json that is not
 * already present in the `handlers` registry.
 *
 * @param {(endpoint: string, fn: Function, meta?: object) => void} registerHandler
 * @param {Map<string, object>} handlers  the router's live handler registry
 */
function register(registerHandler, handlers) {
  const seen = new Set();
  for (const entry of ENDPOINT_MAP) {
    if (!entry || !entry.endpoint) continue;
    const name = entry.endpoint;
    if (seen.has(name)) continue; // dedupe duplicate endpoint_map rows
    seen.add(name);
    if (handlers.has(name)) continue; // a real handler already owns this endpoint
    registerHandler(name, genericStub, {
      reqType: entry.reqType,
      resType: entry.resType
    });
  }
}

module.exports = { register, genericStub };
