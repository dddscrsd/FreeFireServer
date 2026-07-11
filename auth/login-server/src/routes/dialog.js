'use strict';

const crypto = require('crypto');
const {
  config,
  createRateLimiter,
  asyncHandler,
  signState,
} = require('@auth/shared');
const renderLoginPage = require('../views/login');
const renderFake503 = require('../views/fake-503');

const ALLOWED_REDIRECT_URIS = new Set(['fbconnect://success']);
const DEFAULT_REDIRECT_URI = 'fbconnect://success';
const DISCORD_SCOPES = 'identify email';

// Pairings live for 10 minutes. The client-side poll timeout in
// static/dialog.js should match this — if you change one, change the
// other.
const PAIRING_TTL_SECONDS = 600;

function normalizeRedirectUri(uri) {
  return ALLOWED_REDIRECT_URIS.has(uri) ? uri : DEFAULT_REDIRECT_URI;
}

function generatePairId() {
  // 24 bytes -> 32 URL-safe characters. Unguessable, easy to pass in
  // query strings.
  return crypto.randomBytes(24).toString('base64url');
}

function buildAuthorizeUrl(state) {
  const params = new URLSearchParams({
    response_type: 'code',
    client_id: config.DISCORD_CLIENT_ID,
    scope: DISCORD_SCOPES,
    redirect_uri: config.DISCORD_REDIRECT_URI,
    state,
    prompt: 'none',
  });
  return `https://discord.com/api/oauth2/authorize?${params.toString()}`;
}

module.exports = function registerDialogRoutes(app, { pairings }) {
  const rateLimit = createRateLimiter({
    windowMs: 60_000,
    max: 30,
    blockMs: 5 * 60_000,
  });

  const paths = ['/dialog/oauth', '/v:version/dialog/oauth'];

  const handler = asyncHandler(async (req, res) => {
    const clientId = req.query.client_id;
    const redirectUriRaw = req.query.redirect_uri;

    // Hide the endpoint from casual scanners: without both parameters we
    // return the exact fake unavailability page the original server used.
    if (!clientId || !redirectUriRaw) {
      req.log && req.log.warn(
        { have_client_id: !!clientId, have_redirect_uri: !!redirectUriRaw },
        'dialog probe rejected'
      );
      res.status(503).type('html').send(renderFake503());
      return;
    }

    const redirectUri = normalizeRedirectUri(String(redirectUriRaw));
    const pairId = generatePairId();

    // Reserve a pending pairing row before the browser navigates away.
    // The Discord callback route completes this row; the /pair/status
    // poll drains it.
    await pairings.createPending({
      pairId,
      redirectUri,
      ttlSeconds: PAIRING_TTL_SECONDS,
    });

    const state = signState(
      { redirect_uri: redirectUri, pair_id: pairId },
      { secret: config.OAUTH_STATE_SECRET }
    );
    const authorizeUrl = buildAuthorizeUrl(state);

    req.log && req.log.info(
      { client_id: String(clientId).slice(0, 30), redirect_uri: redirectUri, pair_id: pairId },
      'dialog rendered'
    );

    res.status(200).type('html').send(
      renderLoginPage({ discordAuthorizeUrl: authorizeUrl, pairId })
    );
  });

  for (const p of paths) app.get(p, rateLimit, handler);
};
