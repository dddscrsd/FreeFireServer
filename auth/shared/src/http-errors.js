'use strict';

// The original Flask server returns { "error": "invalid_grant", "code": 2017 }
// with HTTP 200 for every token-authentication failure path. The game client
// relies on that shape and status code, so we preserve both exactly.
function oauthInvalidGrant(res) {
  return res.status(200).json({ error: 'invalid_grant', code: 2017 });
}

// Facebook Graph API-style OAuthException. Used by /me, /me/permissions,
// /debug_token, etc. The Graph error shape uses HTTP 400 for missing tokens
// and HTTP 401 for invalid tokens.
function graphOAuthError(res, { code, message, httpStatus = 401 }) {
  return res.status(httpStatus).json({
    error: {
      message,
      type: 'OAuthException',
      code,
    },
  });
}

// Wrap an async Express handler so unhandled rejections propagate to the
// global error handler instead of hanging the request.
function asyncHandler(fn) {
  return function wrappedAsyncHandler(req, res, next) {
    Promise.resolve(fn(req, res, next)).catch(next);
  };
}

module.exports = { oauthInvalidGrant, graphOAuthError, asyncHandler };
