'use strict';

const crypto = require('crypto');

const ACCESS_TTL_SECONDS = 1_296_000;   // 15 days
const REFRESH_TTL_SECONDS = 2_592_000;  // 30 days

// crypto.randomBytes(48).toString('hex') = 96 characters, comfortably above
// the 64-character minimum requirement and matching the original server's
// >= 64-character tokens.
function generateToken() {
  return crypto.randomBytes(48).toString('hex');
}

function nowSeconds() {
  return Math.floor(Date.now() / 1000);
}

// Issues a brand new access + refresh token pair. Used for fresh logins.
async function issueTokens(guests, openId, { ip = null, now = nowSeconds() } = {}) {
  const access = generateToken();
  const refresh = generateToken();
  const access_expiry = now + ACCESS_TTL_SECONDS;
  const refresh_expiry = now + REFRESH_TTL_SECONDS;
  await guests.setTokens(openId, {
    access,
    refresh,
    access_expiry,
    refresh_expiry,
    create_time: now,
    last_login: new Date(now * 1000).toISOString(),
    last_ip: ip,
  });
  return {
    access_token: access,
    refresh_token: refresh,
    access_expiry,
    refresh_expiry,
    create_time: now,
  };
}

// Rotates only the access token, leaves the refresh token untouched. Used
// for the /oauth/token refresh flow.
async function refreshAccessToken(guests, openId, { now = nowSeconds() } = {}) {
  const access = generateToken();
  const access_expiry = now + ACCESS_TTL_SECONDS;
  await guests.refreshAccessToken(openId, { access, access_expiry });
  return { access_token: access, access_expiry };
}

// Returns { ok, user, reason }. `reason` is one of:
//   'missing' | 'unknown' | 'renovation' | 'expired'
// A renovation-blocked lookup also clears the tokens so the client cannot
// continue using them silently.
async function validateAccessToken(guests, token, { now = nowSeconds() } = {}) {
  if (!token || typeof token !== 'string') return { ok: false, reason: 'missing' };
  const user = await guests.findByAccessToken(token);
  if (!user) return { ok: false, reason: 'unknown' };
  if (user.renovation === true) {
    await guests.clearTokens(user.open_id);
    return { ok: false, reason: 'renovation' };
  }
  if (typeof user.access_expiry !== 'number' || now > user.access_expiry) {
    return { ok: false, reason: 'expired' };
  }
  return { ok: true, user };
}

async function validateRefreshToken(guests, token, { now = nowSeconds() } = {}) {
  if (!token || typeof token !== 'string') return { ok: false, reason: 'missing' };
  const user = await guests.findByRefreshToken(token);
  if (!user) return { ok: false, reason: 'unknown' };
  if (user.renovation === true) {
    await guests.clearTokens(user.open_id);
    return { ok: false, reason: 'renovation' };
  }
  if (typeof user.refresh_expiry !== 'number' || now > user.refresh_expiry) {
    return { ok: false, reason: 'expired' };
  }
  return { ok: true, user };
}

module.exports = {
  generateToken,
  issueTokens,
  refreshAccessToken,
  validateAccessToken,
  validateRefreshToken,
  ACCESS_TTL_SECONDS,
  REFRESH_TTL_SECONDS,
  nowSeconds,
};
