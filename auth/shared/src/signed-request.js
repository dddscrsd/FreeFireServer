'use strict';

const crypto = require('crypto');

function base64UrlNoPad(buf) {
  return buf.toString('base64').replace(/=+$/g, '').replace(/\+/g, '-').replace(/\//g, '_');
}

function base64UrlDecode(str) {
  let padded = str.replace(/-/g, '+').replace(/_/g, '/');
  while (padded.length % 4 !== 0) padded += '=';
  return Buffer.from(padded, 'base64');
}

// Build a Facebook-style signed_request in the format:
//   base64url(hmac_sha256(secret, encoded)) + "." + encoded
// where `encoded` is base64url(JSON(payload)).
function buildSignedRequest({ uid, accessToken, appId, appSecret, issuedAt }) {
  const payload = {
    algorithm: 'HMAC-SHA256',
    issued_at: issuedAt,
    user_id: uid,
    code: accessToken,
    app_id: appId,
    page: { id: null, admin: false, liked: false },
  };
  const encoded = base64UrlNoPad(Buffer.from(JSON.stringify(payload), 'utf8'));
  const sig = crypto.createHmac('sha256', appSecret).update(encoded).digest();
  return `${base64UrlNoPad(sig)}.${encoded}`;
}

// Returns the parsed payload on success, or null on any verification failure.
// Intended for tests; the server itself never receives signed_requests from
// clients, it only produces them.
function verifySignedRequest(signedRequest, appSecret) {
  if (typeof signedRequest !== 'string') return null;
  const idx = signedRequest.indexOf('.');
  if (idx < 0) return null;
  const sigPart = signedRequest.slice(0, idx);
  const encoded = signedRequest.slice(idx + 1);
  const provided = base64UrlDecode(sigPart);
  const expected = crypto.createHmac('sha256', appSecret).update(encoded).digest();
  if (provided.length !== expected.length) return null;
  if (!crypto.timingSafeEqual(provided, expected)) return null;
  try {
    return JSON.parse(base64UrlDecode(encoded).toString('utf8'));
  } catch {
    return null;
  }
}

module.exports = { buildSignedRequest, verifySignedRequest, base64UrlNoPad, base64UrlDecode };
