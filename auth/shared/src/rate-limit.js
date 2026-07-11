'use strict';

// Simple fixed-window rate limiter with an in-memory Map. Each key holds
// the request count for the current window plus a `blockedUntil` timestamp
// applied once the client trips the limit. When a request would exceed the
// limit, the middleware responds 429 with `Retry-After`.
//
// This limiter is in-process. A multi-instance deployment should replace
// this with a Redis-backed limiter before enabling more than one replica
// per service.

function defaultKeyer(req) {
  return req.ip || 'unknown';
}

function createRateLimiter({ windowMs, max, blockMs, keyer = defaultKeyer } = {}) {
  if (!Number.isFinite(windowMs) || windowMs <= 0) throw new TypeError('windowMs must be a positive number');
  if (!Number.isFinite(max) || max <= 0) throw new TypeError('max must be a positive number');
  if (!Number.isFinite(blockMs) || blockMs <= 0) throw new TypeError('blockMs must be a positive number');

  const store = new Map();

  function prune() {
    const now = Date.now();
    for (const [key, entry] of store) {
      const windowEnd = entry.windowStart + windowMs;
      const stillBlocked = entry.blockedUntil > now;
      if (!stillBlocked && windowEnd < now) store.delete(key);
    }
  }

  // Prune at most every minute; the interval is unref-ed so it does not
  // keep the event loop alive on shutdown.
  const pruneIntervalMs = Math.max(windowMs, 60_000);
  const intervalHandle = setInterval(prune, pruneIntervalMs);
  if (typeof intervalHandle.unref === 'function') intervalHandle.unref();

  function respond429(req, res, retryAfterMs) {
    const retryAfter = Math.max(1, Math.ceil(retryAfterMs / 1000));
    res.set('Retry-After', String(retryAfter));
    if (req.log && typeof req.log.warn === 'function') {
      req.log.warn({ key: keyer(req), retry_after: retryAfter }, 'rate_limited');
    }
    return res.status(429).json({ error: 'rate_limited', retry_after: retryAfter });
  }

  const middleware = function rateLimit(req, res, next) {
    const key = keyer(req);
    const now = Date.now();

    let entry = store.get(key);
    if (entry && entry.blockedUntil > now) {
      return respond429(req, res, entry.blockedUntil - now);
    }

    if (!entry || entry.windowStart + windowMs < now) {
      entry = { count: 0, windowStart: now, blockedUntil: 0 };
      store.set(key, entry);
    }

    entry.count += 1;
    if (entry.count > max) {
      entry.blockedUntil = now + blockMs;
      return respond429(req, res, blockMs);
    }

    return next();
  };

  middleware.intervalHandle = intervalHandle;
  middleware.storeSize = () => store.size;
  middleware.reset = () => store.clear();
  return middleware;
}

module.exports = { createRateLimiter };
