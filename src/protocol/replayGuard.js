'use strict';

// HTTP request replay guard. Keeps a short-lived, size-bounded set of recently
// seen request signatures (endpoint + exact transport body) and reports a repeat
// within the window as a replay. See config.security.replay for the knobs; it is
// OFF by default.
//
// Why this works despite the static AES key+IV: a genuine client regenerates each
// request, so its varying fields (event_time, sensor_info, storage stats, ...)
// make the ciphertext unique per send. A captured-and-resent packet is byte-for-
// byte identical, so it collides. It does NOT stop a forger who extracted the
// static key and crafts fresh requests — the signature_md5 + auth-token/
// registration gates (see _authGate.js) are what cover that.

const crypto = require('crypto');
const config = require('../../config/default');

const R = (config.security && config.security.replay) || {};
const ENABLED = !!R.enabled;
const WINDOW = R.windowMs || 30000;
const MAX = R.max || 100000;
const EXCLUDE = new Set(R.exclude || []);

// sig -> expiry(ms). Because the window is constant, insertion order == expiry
// order, so we can sweep expired entries from the front of the Map cheaply.
const seen = new Map();
let lastSweep = 0;

function sweep(now) {
  for (const [k, exp] of seen) {
    if (exp <= now) seen.delete(k);
    else break; // front-to-back is oldest-to-newest; stop at the first live entry
  }
  // Hard cap (belt-and-braces against a flood of unique bodies): evict oldest.
  while (seen.size > MAX) {
    const k = seen.keys().next().value;
    if (k === undefined) break;
    seen.delete(k);
  }
}

// True if (endpoint, body) was seen within the window -> this is a replay. A
// fresh request is recorded and returns false. Excluded / empty / disabled -> false.
function isReplay(endpoint, body) {
  if (!ENABLED) return false;
  if (!body || !body.length) return false;
  if (EXCLUDE.has(endpoint)) return false;

  const now = Date.now();
  if (now - lastSweep > 1000) { sweep(now); lastSweep = now; }

  const sig = crypto.createHash('sha256').update(endpoint).update('\0').update(body).digest('hex');
  const exp = seen.get(sig);
  if (exp !== undefined) {
    if (exp > now) return true;   // still within the window -> replay
    seen.delete(sig);            // expired -> drop so re-insert restores order
  }
  seen.set(sig, now + WINDOW);
  if (seen.size > MAX) sweep(now);
  return false;
}

module.exports = { isReplay, enabled: ENABLED };
