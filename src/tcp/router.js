'use strict';

/**
 * TCP message router. Client app messages arrive as a ProtoReq on a frame whose
 * cmd is the base protocol (tcp.EProtocol.Proto, e.g. 3 = MATCHMAKING); the
 * ProtoReq.cmd is the per-protocol sub-command (e.g. tcp.EMatchmaking.Proto). A
 * handler is keyed by (protocol, subcmd).
 *
 * Handler modules live in src/tcp/handlers/*.js and export:
 *   { protocol, subcmd, reqType, resType, handler(reqObj, ctx) -> resObj }
 * (files starting with '_' are skipped; a module may also export an array).
 */

const fs = require('fs');
const path = require('path');
const logger = require('../logger');
const { lookup } = require('../protocol/protos');

const routes = new Map();            // "protocol:subcmd" -> { fn, reqType, resType }
const rkey = (p, s) => `${p}:${s}`;

function register(protocol, subcmd, fn, meta = {}) {
  routes.set(rkey(protocol, subcmd), {
    fn, reqType: meta.reqType, resType: meta.resType, resCmd: meta.resCmd
  });
}

function load() {
  const dir = path.join(__dirname, 'handlers');
  if (!fs.existsSync(dir)) return;
  const files = fs.readdirSync(dir)
    .filter((f) => f.endsWith('.js') && !f.startsWith('_'))
    .sort((a, b) => a.localeCompare(b));
  for (const file of files) {
    const mod = require(path.join(dir, file));
    for (const m of Array.isArray(mod) ? mod : [mod]) {
      if (m && typeof m.handler === 'function' && m.protocol != null && m.subcmd != null) {
        register(m.protocol, m.subcmd, m.handler, { reqType: m.reqType, resType: m.resType, resCmd: m.resCmd });
      } else {
        logger.warn(`[tcp] handler module ${file} missing protocol/subcmd/handler`);
      }
    }
  }
  logger.info(`[tcp] loaded ${routes.size} message route(s)`);
}

/**
 * Dispatch a decoded request. ctx must carry:
 *   { protocol, subcmd, reqData: Buffer, account, region, logger, gateway }
 * Returns { resType, resObj, ret } or null when there is no route.
 */
async function dispatch(ctx) {
  const r = routes.get(rkey(ctx.protocol, ctx.subcmd));
  if (!r) return null;
  let reqObj = {};
  if (r.reqType && ctx.reqData && ctx.reqData.length) {
    const R = lookup(r.reqType);
    if (R) {
      reqObj = R.toObject(R.decode(ctx.reqData), {
        longs: Number, enums: Number, bytes: Buffer, defaults: true
      });
    }
  }
  const resObj = await r.fn(reqObj, ctx);
  // A handler that returns null/undefined is fire-and-forget (no reply). An empty
  // object still replies (e.g. STOP_NTF has empty content).
  if (resObj === null || resObj === undefined) return { noReply: true };
  // resCmd defaults to the request sub-command; handlers whose reply routes under
  // a different sub-command (e.g. START -> START_NTF) set it explicitly.
  return {
    resType: r.resType,
    resObj,
    ret: ctx.ret || 0,
    resCmd: r.resCmd != null ? r.resCmd : ctx.subcmd
  };
}

load();

module.exports = { register, dispatch, routes };
