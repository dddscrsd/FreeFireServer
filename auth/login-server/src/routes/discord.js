'use strict';

const {
  config,
  createRateLimiter,
  asyncHandler,
  verifyState,
} = require('@auth/shared');

const { exchangeCode, fetchUser, DiscordApiError } = require('../discord-client');
const { completeDiscordLogin } = require('../login-flow');
const renderErrorPage = require('../views/error');
const renderCompletePage = require('../views/redirect');

module.exports = function registerDiscordRoute(app, { guests, accounts, pairings }) {
  const rateLimit = createRateLimiter({
    windowMs: 60_000,
    max: 30,
    blockMs: 5 * 60_000,
  });

  // Small helper: mark the pairing failed so the WebView polling learns
  // about the failure instead of waiting for the TTL. Non-fatal — swallow
  // errors and log them.
  async function tryMarkFailed(pairId, reason, log) {
    if (!pairId) return;
    try {
      await pairings.markFailed(pairId, { error: reason });
    } catch (err) {
      (log || console).warn && log.warn({ err, pair_id: pairId }, 'markFailed threw');
    }
  }

  app.get(config.DISCORD_CALLBACK_PATH, rateLimit, asyncHandler(async (req, res) => {
    // Case: user cancelled from Discord's authorize screen. We do not have
    // a validated pair_id yet (state came through, we just haven't
    // verified it), so we cannot mark a specific pairing failed here.
    // The WebView will time out on its polling loop.
    if (req.query.error) {
      req.log && req.log.warn(
        { error: String(req.query.error) },
        'discord authorize cancelled by user'
      );
      return res.status(400).type('html').send(renderErrorPage({
        title: 'Sign-in cancelled',
        message: 'You did not complete the Discord authorization. Return to the game and try again.',
      }));
    }

    const code = typeof req.query.code === 'string' ? req.query.code : '';
    const state = typeof req.query.state === 'string' ? req.query.state : '';

    if (!code || !state) {
      return res.status(400).type('html').send(renderErrorPage({
        title: 'Missing parameters',
        message: 'The Discord callback did not include the expected parameters.',
      }));
    }

    // Verify the signed state (binds this callback to the /dialog/oauth
    // request that started the flow). On failure the pairing — if any —
    // will be cleaned up by TTL.
    const stateResult = verifyState(state, { secret: config.OAUTH_STATE_SECRET });
    if (!stateResult.ok) {
      req.log && req.log.warn({ reason: stateResult.reason }, 'oauth state verification failed');
      return res.status(400).type('html').send(renderErrorPage({
        title: 'Invalid session',
        message: 'The sign-in link expired or could not be verified. Start again from the game client.',
      }));
    }
    const redirectUri = stateResult.redirect_uri;
    const pairId = stateResult.pair_id;

    // From this point on we own a pair_id and can propagate failures back
    // to the polling WebView via markFailed.
    if (!pairId) {
      req.log && req.log.warn('state verified but pair_id missing');
      return res.status(400).type('html').send(renderErrorPage({
        title: 'Invalid session',
        message: 'The sign-in link is not valid for this flow. Start again from the game client.',
      }));
    }

    // Confirm the pairing row still exists in a pending state. It could
    // have been TTL'd out if the user left the flow for >10 minutes.
    const existing = await pairings.findByPairId(pairId);
    if (!existing || existing.status !== 'pending') {
      req.log && req.log.warn({ pair_id: pairId, status: existing && existing.status }, 'pairing not pending on callback');
      return res.status(400).type('html').send(renderErrorPage({
        title: 'Sign-in session expired',
        message: 'Too much time passed between opening the sign-in page and finishing it. Return to the game and try again.',
      }));
    }

    // Exchange the authorization code and fetch the user profile. Discord
    // tokens are used only inside this block and never persisted.
    let discordUser;
    try {
      const { accessToken: discordAccessToken } = await exchangeCode(code);
      discordUser = await fetchUser(discordAccessToken);
    } catch (err) {
      if (err instanceof DiscordApiError) {
        req.log && req.log.warn({ stage: err.stage, status: err.status }, 'discord api failed');
      } else {
        req.log && req.log.error({ err }, 'discord flow unexpected error');
      }
      await tryMarkFailed(pairId, 'discord_api', req.log);
      return res.status(502).type('html').send(renderErrorPage({
        title: 'Sign-in temporarily unavailable',
        message: 'We could not reach Discord to complete your sign-in. Please try again in a moment.',
      }));
    }

    // Provision or refresh the local user, apply the renovation gate,
    // and issue our own access + refresh tokens.
    const flow = await completeDiscordLogin({
      discordUser,
      redirectUri,
      ip: req.ip,
      guests,
      accounts,
      log: req.log,
    });

    if (!flow.ok && flow.reason === 'renovation') {
      await tryMarkFailed(pairId, 'renovation', req.log);
      return res.status(403).type('html').send(renderErrorPage({
        title: 'Account blocked',
        message: 'This account is currently blocked from signing in. If you believe this is a mistake, contact an administrator.',
      }));
    }

    if (!flow.ok) {
      req.log && req.log.error({ reason: flow.reason }, 'login flow returned failure');
      await tryMarkFailed(pairId, 'internal', req.log);
      return res.status(500).type('html').send(renderErrorPage({
        title: 'Something went wrong',
        message: 'We could not complete your sign-in. Please try again.',
      }));
    }

    // Deliver the fbconnect URL to the pairing so the WebView polling
    // loop can pick it up and navigate the game.
    const completed = await pairings.markCompleted(pairId, { redirectUrl: flow.redirectUrl });
    if (!completed) {
      // Row disappeared between our findByPairId and markCompleted. Either
      // it TTL'd out at exactly the wrong moment or something raced.
      req.log && req.log.warn({ pair_id: pairId }, 'pairing not pending at markCompleted');
      return res.status(400).type('html').send(renderErrorPage({
        title: 'Sign-in session expired',
        message: 'The sign-in session expired while completing. Return to the game and try again.',
      }));
    }

    req.log && req.log.info({ pair_id: pairId, uid: flow.uid }, 'pairing completed');

    // The user's browser sees a "done" page; the game's WebView will pick
    // up the redirect URL through its polling loop within ~2 seconds.
    return res.status(200).type('html').send(renderCompletePage(flow.redirectUrl));
  }));
};
