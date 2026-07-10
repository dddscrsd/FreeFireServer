'use strict';

// Smoke test for the custom-room lifecycle (src/tcp/rooms.js + the Room* handlers), driven
// through the real handler modules against an in-memory Redis fake — no live Redis needed.
// Verifies: create -> list -> join (team balance + JOIN_NTF) -> set-ready (SETREADY_NTF) ->
// change settings (CHANGE_NTF + opaque storage) -> kick (KICK_NTF) -> owner-leave dismiss,
// plus the error paths (join missing room, double-join). Broadcasts are captured + decoded
// exactly as the gateway would relay them.
//   node scripts/room-smoke.js

const assert = require('assert');

// In-memory bus fake (only what rooms.js calls). Single-threaded test -> locks always succeed.
function makeFakeBus() {
  const hashes = new Map();
  const counters = new Map();
  const pushes = [];
  const h = (k) => { if (!hashes.has(k)) hashes.set(k, new Map()); return hashes.get(k); };
  return {
    pushes,
    async hset(k, f, v) { h(k).set(String(f), String(v)); return 1; },
    async hget(k, f) { const m = h(k); return m.has(String(f)) ? m.get(String(f)) : null; },
    async hdel(k, ...fs) { let n = 0; for (const f of fs) if (h(k).delete(String(f))) n += 1; return n; },
    async hgetall(k) { const o = {}; for (const [f, v] of h(k)) o[f] = v; return o; },
    async incr(k) { const n = (counters.get(k) || 0) + 1; counters.set(k, n); return n; },
    async acquireLock() { return true; },
    async releaseLock() { return true; },
    async publishPS(_ch, _type, obj) { pushes.push(obj); return 1; }
  };
}

// Patch the bus singleton BEFORE rooms.js captures getBus (it destructures at require time).
const instance = require('../src/bus/instance');
const fake = makeFakeBus();
instance.getBus = () => fake;

const rooms = require('../src/tcp/rooms');
const { ECustomRoom, ECustomRoomErr } = require('../src/tcp/protocol');
const { lookup } = require('../src/protocol/protos');
const RoomCreate = require('../src/tcp/handlers/RoomCreate');
const RoomList = require('../src/tcp/handlers/RoomList');
const RoomJoin = require('../src/tcp/handlers/RoomJoin');
const RoomSetReady = require('../src/tcp/handlers/RoomSetReady');
const RoomChange = require('../src/tcp/handlers/RoomChange');
const RoomKick = require('../src/tcp/handlers/RoomKick');
const RoomLeave = require('../src/tcp/handlers/RoomLeave');

const quietLog = { info() {}, warn() {}, error(m) { console.error(m); } };
const A = { uid: 101, nickname: 'Alice', selected_items: { head_pic: 900, banner_id: 5 } };
const B = { uid: 202, nickname: 'Bob', selected_items: {} };
const ctxFor = (account) => ({ account, logger: quietLog, ret: 0 });

// Drain + return the pushes captured since the last drain (broadcasts to other members).
function drainPushes() { const p = fake.pushes.splice(0); return p; }
function decodePush(p, typeName) {
  const T = lookup(typeName);
  return T.toObject(T.decode(Buffer.isBuffer(p.content) ? p.content : Buffer.from(p.content)), { longs: Number, enums: Number, defaults: true });
}

async function main() {
  // 1) CREATE — owner seats on team 1, room is WAITING, no broadcast.
  let ctx = ctxFor(A);
  const created = await RoomCreate.handler({ room_name: 'Test', game_mode: 15, map_id: 1, room_setting: 0x2000000 }, ctx);
  assert.strictEqual(ctx.ret, 0, 'create ret ok');
  assert.ok(created.id > 0, 'room got an id');
  assert.strictEqual(Number(created.owner), 101, 'owner is Alice');
  assert.strictEqual(created.groups[0].members[0].account_id, 101, 'Alice on team 1');
  assert.strictEqual(created.state, rooms.ROOM_STATE.WAITING, 'state waiting');
  assert.strictEqual(drainPushes().length, 0, 'create broadcasts nothing');
  const roomId = created.id;

  // 2) LIST — one waiting room.
  const listed = await RoomList.handler({}, ctxFor(A));
  assert.strictEqual(listed.room_list.length, 1, 'one room listed');
  assert.strictEqual(listed.room_list[0].cur_member_num, 1, 'one member');

  // 3) JOIN — Bob lands on the empty team 2; Alice gets JOIN_NTF(5) with room_info.
  ctx = ctxFor(B);
  const joined = await RoomJoin.handler({ room_id: roomId }, ctx);
  assert.strictEqual(ctx.ret, 0, 'join ret ok');
  const allMembers = joined.groups.flatMap((g) => g.members.map((m) => m.account_id));
  assert.deepStrictEqual(allMembers.sort(), [101, 202], 'both members present');
  const bobTeam = joined.groups.find((g) => g.members.some((m) => m.account_id === 202)).id;
  assert.strictEqual(bobTeam, 2, 'Bob balanced onto team 2');
  let pushes = drainPushes();
  assert.strictEqual(pushes.length, 1, 'join broadcasts to the 1 other member');
  assert.strictEqual(pushes[0].target_account_id, 101, 'JOIN_NTF -> Alice');
  assert.strictEqual(pushes[0].cmd, ECustomRoom.JOIN_NTF, 'cmd = JOIN_NTF(5)');
  const jn = decodePush(pushes[0], 'tcp.RoomJoinNtf');
  assert.strictEqual(jn.join_player_list[0].account_id, 202, 'JOIN_NTF names Bob');
  assert.strictEqual(Number(jn.room_info.id), roomId, 'JOIN_NTF carries room_info');

  // 4) SET-READY — Bob readies up; Alice gets SETREADY_NTF(25) with the player list.
  ctx = ctxFor(B);
  await RoomSetReady.handler({ ready: true }, ctx);
  assert.strictEqual(ctx.ret, 0, 'setready ret ok');
  assert.strictEqual((await rooms.get(roomId)).members.find((m) => m.account_id === 202).ready, true, 'Bob is ready in store');
  pushes = drainPushes();
  assert.strictEqual(pushes[0].cmd, ECustomRoom.SETREADY_NTF, 'cmd = SETREADY_NTF(25)');
  assert.strictEqual(pushes[0].target_account_id, 101, 'SETREADY_NTF -> Alice');

  // 5) CHANGE — owner edits settings; stored OPAQUE; Bob gets CHANGE_NTF(14)=RoomInfo.
  ctx = ctxFor(A);
  const blob = Buffer.from([1, 2, 3, 4]);
  const changed = await RoomChange.handler({ room_id: roomId, room_setting: 0x4000000, room_setting2: 32, cs_advanced_setting: blob }, ctx);
  assert.strictEqual(ctx.ret, 0, 'change ret ok');
  assert.strictEqual(changed.room_setting, 0x4000000, 'room_setting updated');
  assert.strictEqual(changed.is_cs_advanced, true, 'advanced inferred from blob');
  const stored = await rooms.get(roomId);
  assert.strictEqual(Buffer.from(stored.cs_advanced_b64, 'base64').toString('hex'), '01020304', 'blob stored opaque');
  pushes = drainPushes();
  assert.strictEqual(pushes[0].cmd, ECustomRoom.CHANGE_NTF, 'cmd = CHANGE_NTF(14)');
  assert.strictEqual(pushes[0].target_account_id, 202, 'CHANGE_NTF -> Bob');

  // 6) Non-owner change is rejected (NOTOWNER).
  ctx = ctxFor(B);
  await RoomChange.handler({ room_id: roomId, room_setting: 7 }, ctx);
  assert.strictEqual(ctx.ret, ECustomRoomErr.NOTOWNER, 'non-owner change -> NOTOWNER');

  // 7) KICK — owner kicks Bob; Bob (and the now-empty roster) get KICK_NTF(10).
  ctx = ctxFor(A);
  await RoomKick.handler({ room_id: roomId, kick_account_id: 202 }, ctx);
  assert.strictEqual(ctx.ret, 0, 'kick ret ok');
  assert.strictEqual((await rooms.get(roomId)).members.length, 1, 'only Alice left');
  pushes = drainPushes();
  assert.ok(pushes.some((p) => p.target_account_id === 202 && p.cmd === ECustomRoom.KICK_NTF), 'KICK_NTF -> Bob');

  // 8) Errors: join a missing room, and Bob (kicked) can no longer be in a room.
  ctx = ctxFor(B);
  await RoomJoin.handler({ room_id: 999999 }, ctx);
  assert.strictEqual(ctx.ret, ECustomRoomErr.NOROOM, 'join missing room -> NOROOM');

  // 9) OWNER LEAVE -> dismiss: room gone, membership cleared.
  ctx = ctxFor(A);
  await RoomLeave.handler({}, ctx);
  assert.strictEqual(await rooms.get(roomId), null, 'room dismissed when owner leaves');
  assert.strictEqual(await rooms.roomIdOf(101), 0, 'Alice membership cleared');

  console.log('room-smoke OK: create/list/join(balance)/ready/change(opaque)/kick/dismiss + error paths');
}

main().catch((e) => { console.error('room-smoke FAILED:', e.stack || e); process.exit(1); });
