'use strict';

/**
 * Minimal HS256 JWT (sign/verify) built on Node's crypto — no external deps.
 *
 * The match `prepare_token` is one of these: the matchmaker signs it with the
 * shared MATCH_JWT_SECRET and the Go match-server (match-server/token/jwt.go)
 * verifies it with the same secret. The two implementations MUST stay wire
 * compatible: base64url (no padding) segments, HMAC-SHA256 over
 * `header.payload`, standard `{"alg":"HS256","typ":"JWT"}` header.
 */

const crypto = require('crypto');

function b64url(buf) {
  return Buffer.from(buf)
    .toString('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');
}

function b64urlJson(obj) {
  return b64url(Buffer.from(JSON.stringify(obj), 'utf8'));
}

function b64urlDecode(seg) {
  return Buffer.from(seg.replace(/-/g, '+').replace(/_/g, '/'), 'base64');
}

/**
 * Sign `payload` (a plain object) into a compact JWT string.
 * `iat` is stamped automatically; pass { expiresInSec } to add `exp`.
 */
function sign(payload, secret, opts) {
  const options = opts || {};
  const header = { alg: 'HS256', typ: 'JWT' };
  const now = Math.floor(Date.now() / 1000);
  const body = Object.assign({ iat: now }, payload);
  if (options.expiresInSec) body.exp = now + options.expiresInSec;
  const signingInput = `${b64urlJson(header)}.${b64urlJson(body)}`;
  const sig = crypto.createHmac('sha256', secret).update(signingInput).digest();
  return `${signingInput}.${b64url(sig)}`;
}

/**
 * Verify a JWT and return its decoded payload. Throws on a malformed token, a
 * bad signature, or an expired `exp`.
 */
function verify(token, secret) {
  const parts = String(token).split('.');
  if (parts.length !== 3) throw new Error('jwt: malformed token');
  const signingInput = `${parts[0]}.${parts[1]}`;
  const expected = b64url(crypto.createHmac('sha256', secret).update(signingInput).digest());
  const got = Buffer.from(parts[2]);
  const exp = Buffer.from(expected);
  if (got.length !== exp.length || !crypto.timingSafeEqual(got, exp)) {
    throw new Error('jwt: bad signature');
  }
  const payload = JSON.parse(b64urlDecode(parts[1]).toString('utf8'));
  if (payload.exp && Math.floor(Date.now() / 1000) > payload.exp) {
    throw new Error('jwt: token expired');
  }
  return payload;
}

module.exports = { sign, verify };
