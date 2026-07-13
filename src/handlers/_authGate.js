'use strict';

// Login/register hardening gate. Centralises the checks the game login endpoints
// run before admitting a client, and turns a rejection into the transport action
// the user asked for (HTTP 403 + optional cross-process TCP kick + force-close).
//
// Every gate is config-driven and OFF/safe by default (see config.auth) so the
// live client — including the deterministic dev guest — keeps working until an
// operator opts in. Decisions look like { status, reason, kick?, forceClose? }.
//
//   const d = await gate.runLoginGate(reqObj, openId);   // version + signature + registration
//   if (d) return gate.reject(ctx, d, uid);              // 403 [+ kick] [+ force-close]
//   ... then, once the account is loaded ...
//   const b = await gate.checkBan(openId, account);
//   if (b) return gate.reject(ctx, b, account.uid);

const config = require('../../config/default');
const logger = require('../logger');
const authStore = require('../db/authStore');
const { getBus } = require('../bus/instance');

const A = config.auth || {};

// --- individual checks (return a decision, or null when the gate passes) ------

function checkVersion(reqObj) {
  const allowed = A.allowedVersions || [];
  if (!allowed.length) return null; // gate off
  const v = String((reqObj && reqObj.client_version) || '');
  if (allowed.includes(v)) return null;
  return { status: 403, reason: `client_version '${v}' not in allow-list` };
}

function checkSignature(reqObj) {
  const allowed = A.allowedSignatures || [];
  if (!allowed.length) return null; // gate off
  const sig = String((reqObj && reqObj.signature_md5) || '');
  if (allowed.includes(sig)) return null;
  // A forged/unknown client build: refuse AND kick + force-close, per spec.
  return { status: 403, reason: `signature_md5 '${sig}' not in allow-list`, kick: true, forceClose: true };
}

// Only accounts provisioned by the auth server (open_id present in auth.guests)
// may join. Fails OPEN on a DB error so a transient auth-DB blip can't lock out
// every login.
async function checkRegistration(openId, loginToken) {
  if (!A.enforceRegistration) return null;
  if (!authStore.isEnabled()) {
    logger.warn('[authgate] AUTH_ENFORCE_REGISTRATION set but auth store is disabled (no DATABASE_URL) — skipping');
    return null;
  }
  if (A.allowGuest && openId === A.guestOpenId) return null; // dev-guest bypass
  try {
    const g = await authStore.guestByOpenId(openId);
    if (!g) return { status: 403, reason: 'open_id not registered on auth server' };
    if (g.renovation === true) return { status: 403, reason: 'account blocked (renovation)', kick: true };
    if (A.enforceLoginToken) {
      const t = String(loginToken || '');
      if (!t || t !== g.access_token) return { status: 403, reason: 'login_token does not match auth server', kick: true };
    }
    return null;
  } catch (e) {
    logger.warn(`[authgate] registration check DB error (fail-open): ${e.message}`);
    return null;
  }
}

// Ban gate: the game store's own flag (state.banned) OR the auth store
// (auth.accounts.banned / authorized=false). Fails open on a DB error.
async function checkBan(openId, account) {
  if (!A.enforceBans) return null;
  if (account && account.banned === true) {
    return { status: 403, reason: 'account banned (local)', kick: true };
  }
  if (!authStore.isEnabled() || !openId) return null;
  try {
    const a = await authStore.accountByOpenId(openId);
    if (a && (a.banned === true || a.authorized === false)) {
      return { status: 403, reason: a.ban_reason ? `banned: ${a.ban_reason}` : 'account banned', kick: true };
    }
    return null;
  } catch (e) {
    logger.warn(`[authgate] ban check DB error (fail-open): ${e.message}`);
    return null;
  }
}

// version + signature + registration, short-circuiting on the first failure.
async function runLoginGate(reqObj, openId) {
  return (
    checkVersion(reqObj) ||
    checkSignature(reqObj) ||
    (await checkRegistration(openId, reqObj && reqObj.login_token))
  );
}

// --- transport actions --------------------------------------------------------

// Best-effort cross-process TCP kick: publish a SessionRevoke on the bus; the
// gateway holding this uid (if any) sends Cmd=11 + closes the socket. No-op when
// the bus is disabled or the client isn't connected anywhere.
function revoke(uid, reason) {
  if (!uid) return;
  try {
    const bus = getBus();
    if (!bus) return;
    bus
      .publishPS('session.revoke', 'SessionRevoke', { account_id: Number(uid), reason: String(reason || 'revoked') })
      .then(() => logger.info(`[authgate] session.revoke published uid=${uid} (${reason})`))
      .catch((e) => logger.warn(`[authgate] revoke publish: ${e.message}`));
  } catch (e) {
    logger.warn(`[authgate] revoke: ${e.message}`);
  }
}

// Apply a rejection decision to the HTTP request: log, optionally kick the
// connected client, respond with the status (default 403, empty body), and —
// when forceClose is set — destroy the socket. Returns undefined so the handler
// can `return gate.reject(...)` (the router sees headersSent and won't re-encode).
function reject(ctx, decision, uid) {
  const status = decision.status || 403;
  logger.warn(`[authgate] ${ctx.endpoint || ''} rejected uid=${uid || '?'}: ${decision.reason} -> ${status}`);
  if (decision.kick) revoke(uid, decision.reason);
  try {
    ctx.res.status(status).type('text/plain').end();
  } catch (e) {
    logger.warn(`[authgate] reject send failed: ${e.message}`);
  }
  if (decision.forceClose && ctx.res && ctx.res.socket) {
    setTimeout(() => { try { ctx.res.socket.destroy(); } catch (e) { /* ignore */ } }, 50);
  }
  return undefined;
}

module.exports = {
  checkVersion,
  checkSignature,
  checkRegistration,
  checkBan,
  runLoginGate,
  reject,
  revoke,
};
