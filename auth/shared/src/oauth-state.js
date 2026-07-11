'use strict';

const crypto = require('crypto');
const { base64UrlNoPad, base64UrlDecode } = require('./signed-request');

const DEFAULT_MAX_AGE_SECONDS = 600;

// The state parameter binds the Discord OAuth round-trip to the original
// game-client /dialog/oauth request. It carries:
//   - the redirect_uri that the game client asked for,
//   - a random nonce (to make each state unique),
//   - the issue time (to bound how long the callback can be replayed),
//   - optionally the pair_id that ties this OAuth trip back to a pending
//     pairing row in MongoDB.
//
// Wire format:
//   base64url(json({ r, n, t, p? })) + "." + base64url(hmac_sha256(secret, encoded))
//
// `pair_id` is optional at the state module level so tests and non-pairing
// flows do not have to construct one. Callers that require it (e.g., the
// login-server callback route) validate its presence themselves.
function signState({ redirect_uri, pair_id }, { secret, now = Math.floor(Date.now() / 1000) } = {}) {
  const body = {
    r: redirect_uri,
    n: crypto.randomBytes(12).toString('base64url'),
    t: now,
  };
  if (pair_id) body.p = pair_id;
  const encoded = base64UrlNoPad(Buffer.from(JSON.stringify(body), 'utf8'));
  const sig = base64UrlNoPad(crypto.createHmac('sha256', secret).update(encoded).digest());
  return `${encoded}.${sig}`;
}

function verifyState(state, { secret, maxAgeSeconds = DEFAULT_MAX_AGE_SECONDS, now = Math.floor(Date.now() / 1000) } = {}) {
  if (typeof state !== 'string') return { ok: false, reason: 'malformed' };
  const idx = state.indexOf('.');
  if (idx < 0) return { ok: false, reason: 'malformed' };
  const encoded = state.slice(0, idx);
  const sigPart = state.slice(idx + 1);

  const provided = base64UrlDecode(sigPart);
  const expected = crypto.createHmac('sha256', secret).update(encoded).digest();
  if (provided.length !== expected.length) return { ok: false, reason: 'signature' };
  if (!crypto.timingSafeEqual(provided, expected)) return { ok: false, reason: 'signature' };

  let body;
  try {
    body = JSON.parse(base64UrlDecode(encoded).toString('utf8'));
  } catch {
    return { ok: false, reason: 'malformed' };
  }

  if (typeof body !== 'object' || body === null) return { ok: false, reason: 'malformed' };
  if (typeof body.t !== 'number' || !Number.isFinite(body.t)) return { ok: false, reason: 'malformed' };
  if (typeof body.r !== 'string' || body.r.length === 0) return { ok: false, reason: 'malformed' };
  if (now - body.t > maxAgeSeconds) return { ok: false, reason: 'expired' };

  const pair_id = typeof body.p === 'string' && body.p.length > 0 ? body.p : null;
  return { ok: true, redirect_uri: body.r, pair_id };
}

module.exports = { signState, verifyState };
