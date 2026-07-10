'use strict';

// Smoke test for the matchmaker HA leader-election lease (Bus.acquireLock / renewLock /
// releaseLock), the primitives src/tcp/matchmaker.js uses to run N replicas with exactly
// one ACTIVE processor + automatic failover. Runs the REAL Bus methods against a tiny
// in-memory Redis fake (SET NX PX + the two CAS Lua scripts), so no live Redis is needed.
//   node scripts/leader-smoke.js

const assert = require('assert');
const { Bus } = require('../src/bus');

// Minimal Redis fake: a virtual clock drives TTL expiry deterministically. Implements only
// what the lock methods call — set(key,val,PX,ttl,NX) and eval() of the renew/release scripts.
function makeFakeRedis(clock) {
  const store = new Map(); // key -> { val, exp(ms-epoch|Infinity) }
  const live = (k) => {
    const e = store.get(k);
    if (!e) return null;
    if (e.exp <= clock.now) { store.delete(k); return null; }
    return e;
  };
  return {
    _store: store,
    async set(key, val, ...opts) {
      let px = null; let nx = false;
      for (let i = 0; i < opts.length; i += 1) {
        const o = String(opts[i]).toUpperCase();
        if (o === 'PX') { px = Number(opts[i + 1]); i += 1; }
        else if (o === 'EX') { px = Number(opts[i + 1]) * 1000; i += 1; }
        else if (o === 'NX') { nx = true; }
      }
      if (nx && live(key)) return null;            // NX: fail if a live key exists
      store.set(key, { val, exp: px != null ? clock.now + px : Infinity });
      return 'OK';
    },
    // Emulate the two CAS scripts by intent: 'pexpire' => renew, 'del' => release.
    async eval(script, _numKeys, key, ...argv) {
      const [token, ttl] = argv;
      const e = live(key);
      const held = !!e && e.val === token;
      if (script.includes('pexpire')) {           // renewLock
        if (!held) return 0;
        e.exp = clock.now + Number(ttl);
        return 1;
      }
      if (!held) return 0;                         // releaseLock
      store.delete(key);
      return 1;
    },
  };
}

const KEY = 'mm:leader';
const TTL = 15000;

function main() {
  const clock = { now: 1_000_000 };
  const redis = makeFakeRedis(clock);
  // Two matchmaker replicas share the SAME Redis; each has its own lease token.
  const A = { pub: redis };
  const B = { pub: redis };
  const acquire = (ctx, tok) => Bus.prototype.acquireLock.call(ctx, KEY, tok, TTL);
  const renew = (ctx, tok) => Bus.prototype.renewLock.call(ctx, KEY, tok, TTL);
  const release = (ctx, tok) => Bus.prototype.releaseLock.call(ctx, KEY, tok);

  return (async () => {
    // 1) Exactly one acquires a free lease.
    assert.strictEqual(await acquire(A, 'A'), true, 'A should acquire the free lease');
    assert.strictEqual(await acquire(B, 'B'), false, 'B must NOT acquire while A holds it');

    // 2) The holder renews; a non-holder cannot renew (or steal via renew).
    assert.strictEqual(await renew(A, 'A'), true, 'A should renew its own lease');
    assert.strictEqual(await renew(B, 'B'), false, "B must not renew a lease it doesn't hold");

    // 3) A non-holder cannot release the holder's lease (no CAS bypass).
    assert.strictEqual(await release(B, 'B'), false, 'B must not release A\'s lease');
    assert.strictEqual(await acquire(B, 'B'), false, 'A still holds it after B\'s bogus release');

    // 4) Graceful handover: the holder releases -> a standby acquires immediately.
    assert.strictEqual(await release(A, 'A'), true, 'A releases its lease');
    assert.strictEqual(await acquire(B, 'B'), true, 'B takes over instantly after release');

    // 5) Crash failover: B holds but stops renewing; once the TTL lapses A acquires. B's
    //    late renew then fails (it lost the lease) so it can't resurrect a stale leadership.
    clock.now += TTL + 1; // B never renewed within the lease window
    assert.strictEqual(await acquire(A, 'A'), true, 'A acquires after B\'s lease expires (failover)');
    assert.strictEqual(await renew(B, 'B'), false, 'B\'s stale renew fails — no split-brain');
    assert.strictEqual(await renew(A, 'A'), true, 'A, the new leader, keeps renewing');

    console.log('leader-smoke OK: single-holder, CAS renew/release, graceful + crash failover');
  })();
}

main().catch((e) => { console.error('leader-smoke FAILED:', e.message); process.exit(1); });
