'use strict';

const { createRateLimiter, asyncHandler } = require('@auth/shared');

// The polling endpoint that the WebView hits every couple of seconds while
// the user is off completing Discord OAuth in another browser context.
//
// Responses (all HTTP 200 unless noted):
//   { status: "pending" }
//   { status: "completed", redirect_url: "fbconnect://success#..." }   [single-use]
//   { status: "failed", error: "..." }                                 [single-use]
//   { status: "expired" }                                              [row gone]
//
// The `completed` and `failed` responses delete the underlying pairing row
// atomically, so the next poll after retrieval returns `expired`.
module.exports = function registerPairRoutes(app, { pairings }) {
  const rateLimit = createRateLimiter({
    windowMs: 60_000,
    max: 60,                // one poll every 2s = 30/min; allow 2x margin
    blockMs: 5 * 60_000,
  });

  app.get('/pair/status', rateLimit, asyncHandler(async (req, res) => {
    const pairId = typeof req.query.id === 'string' ? req.query.id : '';
    if (!pairId) {
      return res.status(400).json({ status: 'invalid', error: 'missing_id' });
    }
    const result = await pairings.consumeIfCompleted(pairId);
    return res.status(200).json(result);
  }));
};
