'use strict';

const {
  createRateLimiter,
  asyncHandler,
  graphOAuthError,
  oauthInvalidGrant,
  validateAccessToken,
} = require('@auth/shared');

// The original server exposed each Graph API path at three variants:
//   /me, /v2.5/me, /v:version/me
// The `/v:version/...` pattern already matches /v2.5/... so we register two
// variants per path (unversioned + versioned) and cover both.
function versioned(basePath) {
  return [basePath, `/v:version${basePath}`];
}

const APP_ID_STRING = '100067';
const APPLICATION_NAME = 'Discord OAuth Bridge';

module.exports = function registerGraphRoutes(app, { guests }) {
  const debugTokenLimiter = createRateLimiter({
    windowMs: 60_000,
    max: 30,               // 30 requests per minute per IP
    blockMs: 5 * 60_000,
  });

  function mount(method, paths, ...handlers) {
    for (const p of paths) app[method](p, ...handlers);
  }

  // Middleware: pulls access_token from the query string and hangs the
  // Guest_Collection user document off req.user. Handles the three failure
  // modes:
  //   - missing token   -> 400 OAuthException code 104
  //   - invalid token   -> 401 OAuthException code 190
  //   - renovation/expiry -> 200 { error: 'invalid_grant', code: 2017 }
  //   (matching the original mixed-shape behavior)
  const withUser = asyncHandler(async (req, res, next) => {
    const token = req.query.access_token;
    if (!token) {
      return graphOAuthError(res, {
        code: 104,
        message: 'Missing access_token',
        httpStatus: 400,
      });
    }
    const result = await validateAccessToken(guests, token);
    if (!result.ok) {
      if (result.reason === 'renovation' || result.reason === 'expired') {
        return oauthInvalidGrant(res);
      }
      return graphOAuthError(res, {
        code: 190,
        message: 'Invalid OAuth access token',
        httpStatus: 401,
      });
    }
    req.user = result.user;
    next();
  });

  // ---------------------------------------------------------------------
  // Specific paths — registered before the /:app_id catch-all below.
  // ---------------------------------------------------------------------

  mount('get', versioned('/me'), withUser, (req, res) => {
    const user = req.user;
    const name = user.dc_handle || `User${user.uid}`;
    const email = user.dc_email || `${user.uid}@auth.local`;
    return res.status(200).json({
      id: user.uid,
      name,
      first_name: name,
      last_name: '',
      email,
    });
  });

  mount('get', versioned('/me/permissions'), withUser, (req, res) => {
    return res.status(200).json({
      data: [
        { permission: 'public_profile', status: 'granted' },
        { permission: 'email', status: 'granted' },
        { permission: 'user_friends', status: 'granted' },
      ],
    });
  });

  mount('get', versioned('/me/friends'), withUser, (req, res) => {
    return res.status(200).json({ data: [], summary: { total_count: 0 } });
  });

  mount('get', versioned('/debug_token'), debugTokenLimiter, asyncHandler(async (req, res) => {
    const inputToken = req.query.input_token;
    if (!inputToken) {
      return graphOAuthError(res, {
        code: 104,
        message: 'Missing input_token',
        httpStatus: 400,
      });
    }
    const result = await validateAccessToken(guests, inputToken);
    if (!result.ok) {
      if (result.reason === 'renovation' || result.reason === 'expired') {
        return oauthInvalidGrant(res);
      }
      return res.status(200).json({
        data: {
          is_valid: false,
          error: {
            message: 'Invalid OAuth access token',
            type: 'OAuthException',
            code: 190,
          },
        },
      });
    }
    const user = result.user;
    return res.status(200).json({
      data: {
        app_id: APP_ID_STRING,
        type: 'USER',
        application: APPLICATION_NAME,
        data_access_expires_at: user.access_expiry,
        expires_at: user.access_expiry,
        is_valid: true,
        scopes: ['public_profile', 'email', 'user_friends'],
        user_id: user.uid,
      },
    });
  }));

  // ---------------------------------------------------------------------
  // /:app_id catch-alls — MUST come last. Anything above this would be
  // shadowed by /:app_id otherwise.
  // ---------------------------------------------------------------------

  // /:app_id/activities is more specific and needs to be registered before
  // /:app_id.
  mount('post', versioned('/:app_id/activities'), (req, res) => {
    // The game SDK posts telemetry here; we accept and drop.
    return res.status(200).json({ success: true });
  });

  mount('get', versioned('/:app_id'), (req, res) => {
    // App-config response matching the original server's payload.
    return res.status(200).json({
      supports_implicit_sdk_logging: true,
      gdpv4_nux_enabled: false,
      gdpv4_nux_content: {},
      android_dialog_configs: {},
      android_sdk_error_categories: [],
      id: req.params.app_id,
    });
  });
};
