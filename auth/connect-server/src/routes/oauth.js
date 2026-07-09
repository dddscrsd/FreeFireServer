'use strict';

const {
  createRateLimiter,
  asyncHandler,
  oauthInvalidGrant,
  validateAccessToken,
  validateRefreshToken,
  refreshAccessToken,
} = require('@auth/shared');

// Every response shape in this file mirrors the original Flask connect.py
// exactly so the game client's SDK does not need to be rebuilt.
//
// Notably: token-authentication failures return HTTP 200 with a
// { error: 'invalid_grant', code: 2017 } body rather than a 4xx status.
module.exports = function registerOAuthRoutes(app, { guests }) {
  const rateLimit = createRateLimiter({
    windowMs: 60_000,   // 1 minute
    max: 60,            // 60 requests per minute
    blockMs: 5 * 60_000 // 5-minute block
  });

  // GET /oauth/token/inspect?token=...
  app.get('/oauth/token/inspect', rateLimit, asyncHandler(async (req, res) => {
    const token = req.query.token;
    const result = await validateAccessToken(guests, token);
    if (!result.ok) return oauthInvalidGrant(res);
    const user = result.user;
    return res.status(200).json({
      expiry_time: user.access_expiry,
      uid: Number(user.uid),
      open_id: user.open_id,
      main_active_platform: 4,
      app_id: 100067,
      platform: 4,
      create_time: user.create_time,
      scope: ['get_user_info', 'get_friends', 'payment', 'send_request'],
      login_type: 2,
      login_platform: 4,
    });
  }));

  // POST /oauth/token (form-encoded)
  app.post('/oauth/token', rateLimit, asyncHandler(async (req, res) => {
    const refresh = (req.body && req.body.refresh_token) || req.query.refresh_token;
    // grant_type is not strictly validated; the original server accepts
    // anything and only cares that the refresh_token matches.

    if (!refresh) return oauthInvalidGrant(res);

    const result = await validateRefreshToken(guests, refresh);
    if (!result.ok) return oauthInvalidGrant(res);

    const user = result.user;
    const { access_token, access_expiry } = await refreshAccessToken(guests, user.open_id);

    req.log && req.log.info({ uid: user.uid }, 'access token refreshed');

    return res.status(200).json({
      access_token,
      expiry_time: access_expiry,
      open_id: user.open_id,
      refresh_token: user.refresh_token,
      refresh_expiry_time: user.refresh_expiry,
      token_type: 'Bearer',
      uid: Number(user.uid),
    });
  }));

  // GET /oauth/logout?access_token=...&refresh_token=...
  app.get('/oauth/logout', asyncHandler(async (req, res) => {
    const accessToken = req.query.access_token;
    const refreshToken = req.query.refresh_token;

    // No match, either because the params were missing or the tokens are
    // stale, returns { success: 'true' } (per the original server) rather
    // than leaking whether the tokens were valid.
    if (!accessToken || !refreshToken) {
      return res.status(200).json({ success: 'true' });
    }
    const user = await guests.findByAccessAndRefreshToken(accessToken, refreshToken);
    if (!user) return res.status(200).json({ success: 'true' });

    await guests.clearTokens(user.open_id);
    req.log && req.log.info({ uid: user.uid }, 'user logged out');
    return res.status(200).json({ result: 0 });
  }));

  // POST /oauth/token/facebook/exchange (form-encoded)
  // The original endpoint bridges the fbconnect access token into an
  // OAuth-shaped response for the SDK. Here it just re-serves the stored
  // token pair after validating it.
  app.post('/oauth/token/facebook/exchange', rateLimit, asyncHandler(async (req, res) => {
    const fbToken = req.body && req.body.facebook_access_token;
    if (!fbToken) return oauthInvalidGrant(res);

    const result = await validateAccessToken(guests, fbToken);
    if (!result.ok) return oauthInvalidGrant(res);

    const user = result.user;
    return res.status(200).json({
      access_token: user.access_token,
      code: 0,
      create_time: user.create_time,
      expires_in: 1_296_000,
      expiry_time: user.access_expiry,
      main_active_platform: 4,
      open_id: user.open_id,
      platform: 4,
      refresh_expiry_time: user.refresh_expiry,
      refresh_token: user.refresh_token,
      scope: ['get_user_info', 'get_friends', 'payment', 'send_request'],
      token_type: 'Bearer',
      uid: Number(user.uid),
    });
  }));
};
