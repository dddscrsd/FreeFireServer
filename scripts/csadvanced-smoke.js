'use strict';

// Smoke test for the cs_advanced_setting blob decoder (src/tcp/csadvanced.js). Builds a blob by
// hand (the exact IDA-verified layout) and asserts the decode: indexid -> itemId translation via
// RoomCreate_CSShop.csv, slotKind resolution, price = step*50, quarantine drop, and the raw i32
// per-round table. Also checks that a truncated/empty blob falls back to null.
//   node scripts/csadvanced-smoke.js

const assert = require('assert');
const cs = require('../src/tcp/csadvanced');

// Store section (indexid, priceStep) pairs, chosen to exercise every slotKind branch:
//   5  -> itemId 33  (AN94)   ammo 201 -> primary     step 22 -> 1100
//   3  -> itemId 10  (G18)    ammo 202 -> secondary   step 10 ->  500
//   40 -> itemId 302 (vest2)  filter 2, IT type 4 -> vest        step 8 ->  400
//   48 -> itemId 601 (frag)   filter 4, IT type 8 -> grenade     step 4 ->  200
//   51 -> itemId 1201 (gloo)  filter 4          -> building      step 6 ->  300
//   59 -> itemId 2102 (turret) -> QUARANTINED (dropped)
//   1  -> itemId 51  (sickle) ammo 0  -> melee        step 10 -> 500
const store = [[5, 22], [3, 10], [40, 8], [48, 4], [51, 6], [59, 6], [1, 10]];
// Eco section (eventId, stepIndex): realCoins = min + stepIndex*step, all min=0 step=50 (from the CSV).
//   152 win_round   step 12 -> 600 (host bumped it from the default 500)
//   153 per_kill    step  4 -> 200 (default)
//   157 first_blood step  6 -> 300
const eco = [[152, 12], [153, 4], [157, 6]];
const rounds = [500, 900, 1100];

const bytes = [];
bytes.push(store.length);
for (const [idx, step] of store) bytes.push(idx, step);
bytes.push(eco.length);     // ecoCount
for (const [id, step] of eco) bytes.push(id, step);
bytes.push(rounds.length);  // roundCount
const buf = Buffer.alloc(bytes.length + rounds.length * 4);
Buffer.from(bytes).copy(buf, 0);
let o = bytes.length;
for (const v of rounds) { buf.writeInt32LE(v, o); o += 4; }

const adv = cs.decodeAdvanced(buf);
assert.ok(adv, 'decode returned an overlay');

// round / economy section (raw i32, no scaling)
assert.strictEqual(adv.roundCount, 3, 'roundCount');
assert.deepStrictEqual(adv.perRoundBase, [500, 900, 1100], 'per-round base coin table (raw i32)');

// eco events: stepIndex -> min + step*idx (min 0, step 50 from RoomCreate_CSEco.csv)
assert.ok(adv.events, 'events decoded (RoomCreate_CSEco.csv loaded)');
assert.strictEqual(adv.events.win_round, 600, 'win_round step 12 -> 600');
assert.strictEqual(adv.events.per_kill, 200, 'per_kill step 4 -> 200');
assert.strictEqual(adv.events.first_blood, 300, 'first_blood step 6 -> 300');
assert.strictEqual(cs.ECO_STEP[152].name, 'win_round', 'ECO_STEP maps eventId 152 -> win_round');
assert.strictEqual(cs.ECO_STEP[151], undefined, 'PhasePrepareTime (151, open=false) is not an event');

// quarantine: the turret (2102) is dropped, not offered
assert.deepStrictEqual(adv.dropped, [2102], 'turret dropped from the allow-list');

// shop items: 6 kept (turret dropped), in blob order, translated indexid -> itemId with price = step*50
const byId = Object.fromEntries(adv.shopItems.map((s) => [s.item_id, s]));
assert.strictEqual(adv.shopItems.length, 6, '6 placeable items (7 selected - 1 quarantined)');

assert.deepStrictEqual(byId[33], { item_id: 33, price: 1100, filter: 1, limitation: 0, bonus: false, ammo: 201, slot_kind: 'primary' }, 'AN94 -> primary');
assert.deepStrictEqual(byId[10], { item_id: 10, price: 500, filter: 1, limitation: 0, bonus: false, ammo: 202, slot_kind: 'secondary' }, 'G18 -> secondary');
assert.strictEqual(byId[302].slot_kind, 'vest', 'vest2 -> vest');
assert.strictEqual(byId[302].price, 400, 'vest price = step*50');
assert.strictEqual(byId[601].slot_kind, 'grenade', 'frag -> grenade');
assert.strictEqual(byId[601].limitation, 2, 'frag limitation from CSV');
assert.strictEqual(byId[1201].slot_kind, 'building', 'gloo -> building');
assert.strictEqual(byId[51].slot_kind, 'melee', 'sickle -> melee');
assert.strictEqual(byId[51].ammo, 0, 'sickle has no ammo');

// resolver unit checks (independent of the blob)
assert.strictEqual(cs.resolveSlotKind(6, 201, 1, 0), 'primary', 'AK ammo 201 -> primary');
assert.strictEqual(cs.resolveSlotKind(9, 202, 1, 0), 'secondary', 'Desert Eagle ammo 202 -> secondary');
assert.strictEqual(cs.resolveSlotKind(305, 0, 2, 2), 'helmet', 'helmet (IT type 2) -> helmet');
assert.strictEqual(cs.resolveSlotKind(106, 0, 2, 1), 'consumable', 'repair kit (IT type 1) -> consumable');
assert.strictEqual(cs.resolveSlotKind(709, 0, 4, 1), 'consumable', 'mushroom -> consumable (instant-EP path)');
assert.strictEqual(cs.resolveSlotKind(23, 207, 1, 0), null, 'M79 quarantined -> null');
assert.strictEqual(cs.resolveSlotKind(37, 201, 1, 0), null, 'Gatling quarantined even though CSV ammo=201');
assert.strictEqual(cs.resolveSlotKind(100, 0, 1, 0), null, 'Flamethrower (no ammo) quarantined, not melee');

// malformed / empty blobs fall back to null (start handoff degrades to simple mode)
assert.strictEqual(cs.decodeAdvanced(Buffer.alloc(0)), null, 'empty blob -> null');
assert.strictEqual(cs.decodeAdvanced(Buffer.from([5, 1, 2])), null, 'truncated blob (claims 5 store items) -> null');
assert.strictEqual(cs.decodeAdvanced(null), null, 'non-buffer -> null');

console.log(`csadvanced-smoke OK: layout + indexid->itemId + slotKind + price*50 + quarantine (${cs.SHOP_BY_INDEX.size} shop rows, ${cs.ITEM_TYPE.size} item types loaded)`);
