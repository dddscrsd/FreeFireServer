'use strict';

/**
 * Custom-room (private match) store + wire helpers.
 *
 * Rooms live in REDIS so ANY gateway node can serve ANY member's request (the gateway is
 * pooled). Layout:
 *   `rooms`               hash: roomId -> room-record JSON (the whole room)
 *   `room:members`        hash: accountId -> roomId (reverse lookup for guards + disconnect)
 *   `room:seq`            INCR: globally-unique, restart-safe room ids
 *   `room:lock:<id>`      per-room mutation lease (reuse the bus lock) so concurrent
 *                         joins/leaves to the SAME room can't lose an update (read-modify-write).
 *
 * The record is our own shape; `toRoomInfo` projects it onto the client's `tcp.RoomInfo`.
 * Settings (room_setting / room_setting2 / cs_advanced_setting) are stored OPAQUE — the lobby
 * never decodes the bitfields; it stores + rebroadcasts them (decode happens match-side, later).
 *
 * Broadcasts reach members on ANY node via the presence-routed `gw.push` relay (same path the
 * matchmaker uses for SussNtf), so the requester's own reply comes back on its socket (reliable)
 * while the other members get a best-effort push.
 */

const config = require('../../config/default');
const logger = require('../logger');
const { lookup } = require('../protocol/protos');
const { getBus } = require('../bus/instance');
const { EProtocol, ECustomRoom, ECustomRoomErr } = require('./protocol');

const ROOMS_KEY = 'rooms';
const MEMBERS_KEY = 'room:members';
const SEQ_KEY = 'room:seq';

const ROOM_STATE = { WAITING: 0, STARTING: 1, INGAME: 2 }; // reference tcpp.py CustomRoomInfo.state
const ROLE_MEMBER = 1;    // tcp.ERoom.PlayerRole: NONE=0, MEMBER=1, SPECTATOR=2
const DEFAULT_MAX_MEMBERS = 8; // CS custom default (match-server manager caps at 8 too)
const NUM_TEAMS = 2;           // CS = two groups/teams

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// A room op failed a rule — carries an ECustomRoom.ErrCode the handler puts in MessageNotify.ret.
class RoomError extends Error {
  constructor(code) { super(`room error ${code}`); this.code = code; }
}

// --- store primitives -------------------------------------------------------

async function loadRoom(bus, roomId) {
  const raw = await bus.hget(ROOMS_KEY, String(roomId));
  if (!raw) return null;
  try { return JSON.parse(raw); } catch (_) { return null; }
}

function saveRoom(bus, room) { return bus.hset(ROOMS_KEY, String(room.id), JSON.stringify(room)); }
function deleteRoom(bus, roomId) { return bus.hdel(ROOMS_KEY, String(roomId)); }

// Serialize mutations to ONE room so two gateways can't clobber the JSON blob (read-modify-write).
async function withRoomLock(bus, roomId, fn) {
  const key = `room:lock:${roomId}`;
  const token = `${config.nodeId}#${process.pid}#${Math.random().toString(16).slice(2)}`;
  for (let i = 0; i < 50; i += 1) {
    if (await bus.acquireLock(key, token, 3000)) {
      try { return await fn(); } finally { bus.releaseLock(key, token).catch(() => {}); }
    }
    await sleep(20);
  }
  throw new Error(`room ${roomId} lock timeout`);
}

// --- record <-> proto -------------------------------------------------------

function memberFromAccount(account, groupId, role = ROLE_MEMBER) {
  const sel = account.selected_items || {};
  return {
    account_id: Number(account.uid || account.account_id || 0),
    nickname: account.nickname || 'Player',
    head_pic: sel.head_pic || 0,
    banner_id: sel.banner_id || 0,
    role,
    group_id: groupId,
    ready: false,
    rank: account.rank || 0,
    cs_rank: account.cs_rank || 0
  };
}

function playerInfo(m) {
  return {
    group_id: m.group_id, account_id: m.account_id, nickname: m.nickname,
    head_pic: m.head_pic || 0, banner_id: m.banner_id || 0, ready: true,
    role: m.role || ROLE_MEMBER, rank: m.rank || 0, ranking_points: 0,
    cs_rank: m.cs_rank || 0, cs_ranking_points: 0
  };
}

// Project the record onto tcp.RoomInfo. The client renders ONE team panel per RoomGroupInfo and
// ONE seat per members[] entry — UIRoomBaseController::RefreshUIData -> SetViewData, seats ==
// group.members.Count with NO client-side padding; a RoomPlayerInfo{account_id:0} draws an EMPTY
// seat (SetUIData). So we emit ALL teams and PAD each members[] to the per-team capacity with
// account_id:0 placeholders, else only the host's seat shows and the empty slots are missing.
function toRoomInfo(room) {
  const maxMembers = room.max_member_num || DEFAULT_MAX_MEMBERS;
  const perTeam = Math.max(1, Math.ceil(maxMembers / NUM_TEAMS));
  const byGroup = new Map();
  for (let g = 1; g <= NUM_TEAMS; g += 1) byGroup.set(g, []); // always emit every team
  for (const m of room.members) {
    const g = m.group_id || 1;
    if (!byGroup.has(g)) byGroup.set(g, []);
    byGroup.get(g).push(playerInfo(m));
  }
  const groups = [...byGroup.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([gid, members]) => {
      const seats = members.slice();
      while (seats.length < perTeam) seats.push({ account_id: 0, group_id: gid }); // empty seat
      return { id: gid, name: `Team ${gid}`, abbr_name: `T${gid}`, members: seats };
    });
  return {
    id: room.id, name: room.name, owner: room.owner,
    map_id: room.map_id, game_mode: room.game_mode, group_mode: room.group_mode,
    max_member_num: room.max_member_num, max_spectator_num: room.max_spectator_num,
    state: room.state, code: room.code, groups, spectators: [],
    enable_death_spectate: !!room.enable_death_spectate, room_type: room.room_type,
    match_times: room.match_times || 0, level_visual_style: room.level_visual_style || 0,
    room_setting: room.room_setting || 0, room_setting2: room.room_setting2 || 0,
    is_cs_advanced: !!room.is_cs_advanced,
    cs_advanced_setting: Buffer.from(room.cs_advanced_b64 || '', 'base64'),
    // Both feed UIRoomBaseController::RefreshSystemHint (owner_online checked FIRST):
    // owner_online=false -> "owner offline"; owner_online=true & enough_room_card=false ->
    // "out of room cards". Set both true so the SystemHintLabel stays hidden.
    enough_room_card: true,
    owner_online: true
  };
}

function toBasicInfo(room) {
  return {
    id: room.id, name: room.name, map_id: room.map_id, game_mode: room.game_mode,
    group_mode: room.group_mode, cur_member_num: room.members.length,
    max_member_num: room.max_member_num, cur_spectator_num: 0,
    max_spectator_num: room.max_spectator_num, state: room.state,
    need_code: !!room.code, room_type: room.room_type,
    level_visual_style: room.level_visual_style || 0, is_cs_advanced: !!room.is_cs_advanced,
    room_setting: room.room_setting || 0
  };
}

// Assign a joiner to the emptier team (ties -> team 1) so CS rooms stay balanced without
// requiring the seat-switch op (SWITCHSEAT, a later phase).
function pickTeam(room) {
  const counts = new Array(NUM_TEAMS).fill(0);
  for (const m of room.members) { const i = (m.group_id || 1) - 1; if (i >= 0 && i < NUM_TEAMS) counts[i] += 1; }
  let best = 0;
  for (let i = 1; i < NUM_TEAMS; i += 1) if (counts[i] < counts[best]) best = i;
  return best + 1;
}

function genCode() { return String(Math.floor(100000 + Math.random() * 900000)); }

// --- operations (each returns a plain result; handlers do the wire) ---------

async function create(account, req) {
  const bus = getBus();
  if (!bus) throw new RoomError(ECustomRoomErr.CANNOTCREATEROOM);
  const accountId = Number(account.uid || account.account_id || 0);
  if (await bus.hget(MEMBERS_KEY, String(accountId))) throw new RoomError(ECustomRoomErr.ALREADYINROOM);

  const id = await bus.incr(SEQ_KEY);
  const room = {
    id,
    name: req.room_name || `${account.nickname || 'Player'}'s room`,
    owner: accountId,
    code: req.code || genCode(),
    map_id: req.map_id || 0,
    game_mode: req.game_mode || 0,
    group_mode: req.group_mode || 0,
    max_member_num: req.max_member_num || DEFAULT_MAX_MEMBERS,
    max_spectator_num: req.max_spectator_num || 0,
    state: ROOM_STATE.WAITING,
    room_type: req.room_type || 0,
    enable_death_spectate: !!req.enable_death_spectate,
    level_visual_style: req.level_visual_style || 0,
    room_setting: req.room_setting || 0,
    room_setting2: req.room_setting2 || 0,
    is_cs_advanced: !!req.is_cs_advanced,
    cs_advanced_b64: Buffer.isBuffer(req.cs_advanced_setting) ? req.cs_advanced_setting.toString('base64') : '',
    members: [memberFromAccount(account, 1, ROLE_MEMBER)], // owner seats on team 1
    createdAt: Date.now()
  };
  await saveRoom(bus, room);
  await bus.hset(MEMBERS_KEY, String(accountId), String(id));
  logger.info(`[room] CREATE id=${id} owner=${accountId} code=${room.code} mode=${room.game_mode} adv=${room.is_cs_advanced}`);
  return room;
}

async function list(_req) {
  const bus = getBus();
  if (!bus) return [];
  const all = await bus.hgetall(ROOMS_KEY);
  const out = [];
  for (const raw of Object.values(all || {})) {
    let room; try { room = JSON.parse(raw); } catch (_) { continue; }
    if (room.state === ROOM_STATE.WAITING) out.push(toBasicInfo(room));
  }
  return out;
}

async function join(account, req) {
  const bus = getBus();
  if (!bus) throw new RoomError(ECustomRoomErr.NOROOM);
  const accountId = Number(account.uid || account.account_id || 0);

  // Resolve the target room (by id, else by join-code) BEFORE taking its lock.
  let roomId = Number(req.room_id || 0);
  if (!roomId && req.code) {
    const all = await bus.hgetall(ROOMS_KEY);
    for (const raw of Object.values(all || {})) {
      let r; try { r = JSON.parse(raw); } catch (_) { continue; }
      if (r.code && r.code === String(req.code)) { roomId = r.id; break; }
    }
  }
  if (!roomId) throw new RoomError(ECustomRoomErr.NOROOM);

  return withRoomLock(bus, roomId, async () => {
    const room = await loadRoom(bus, roomId);
    if (!room) throw new RoomError(ECustomRoomErr.NOROOM);
    if (room.state !== ROOM_STATE.WAITING) throw new RoomError(ECustomRoomErr.ROOMINGAME);
    if (req.code && room.code && String(req.code) !== room.code) throw new RoomError(ECustomRoomErr.INVALIDCODE);
    if (room.members.some((m) => m.account_id === accountId)) throw new RoomError(ECustomRoomErr.ALREADYINROOM);
    if (room.members.length >= room.max_member_num) throw new RoomError(ECustomRoomErr.REACHMAXMEMBERS);

    const joiner = memberFromAccount(account, pickTeam(room), ROLE_MEMBER);
    room.members.push(joiner);
    await saveRoom(bus, room);
    await bus.hset(MEMBERS_KEY, String(accountId), String(room.id));
    logger.info(`[room] JOIN id=${room.id} uid=${accountId} team=${joiner.group_id} (${room.members.length}/${room.max_member_num})`);
    return { room, joiner };
  });
}

// Remove an account from whatever room it is in (LEAVE, or a disconnect). If the OWNER
// leaves, the room is DISMISSED (returned dismissed=true with the full member list so the
// caller can notify everyone). Returns null if the account was in no room.
async function leave(accountId) {
  const bus = getBus();
  if (!bus) return null;
  accountId = Number(accountId);
  const roomId = Number(await bus.hget(MEMBERS_KEY, String(accountId)) || 0);
  if (!roomId) return null;

  return withRoomLock(bus, roomId, async () => {
    const room = await loadRoom(bus, roomId);
    await bus.hdel(MEMBERS_KEY, String(accountId));
    if (!room) return null;
    const idx = room.members.findIndex((m) => m.account_id === accountId);
    const leaver = idx >= 0 ? room.members[idx] : null;

    if (room.owner === accountId || room.members.length <= 1) {
      // Owner left (or last member) -> dismiss the whole room.
      const members = room.members.slice();
      await deleteRoom(bus, room.id);
      for (const m of members) if (m.account_id !== accountId) await bus.hdel(MEMBERS_KEY, String(m.account_id));
      logger.info(`[room] DISMISS id=${room.id} (owner/last ${accountId} left)`);
      return { room, leaver, dismissed: true, members };
    }

    if (idx >= 0) room.members.splice(idx, 1);
    await saveRoom(bus, room);
    logger.info(`[room] LEAVE id=${room.id} uid=${accountId} (${room.members.length} left)`);
    return { room, leaver, dismissed: false };
  });
}

async function kick(ownerId, roomId, targetId) {
  const bus = getBus();
  if (!bus) throw new RoomError(ECustomRoomErr.NOROOM);
  ownerId = Number(ownerId); targetId = Number(targetId); roomId = Number(roomId);

  return withRoomLock(bus, roomId, async () => {
    const room = await loadRoom(bus, roomId);
    if (!room) throw new RoomError(ECustomRoomErr.NOROOM);
    if (room.owner !== ownerId) throw new RoomError(ECustomRoomErr.NOTOWNER);
    if (targetId === ownerId) throw new RoomError(ECustomRoomErr.CANNOTKICK);
    const idx = room.members.findIndex((m) => m.account_id === targetId);
    if (idx < 0) throw new RoomError(ECustomRoomErr.NOTINROOM);
    const target = room.members[idx];
    room.members.splice(idx, 1);
    await saveRoom(bus, room);
    await bus.hdel(MEMBERS_KEY, String(targetId));
    logger.info(`[room] KICK id=${room.id} owner=${ownerId} target=${targetId}`);
    return { room, target };
  });
}

async function change(account, req) {
  const bus = getBus();
  if (!bus) throw new RoomError(ECustomRoomErr.NOROOM);
  const accountId = Number(account.uid || account.account_id || 0);
  const roomId = Number(req.room_id || 0);

  return withRoomLock(bus, roomId, async () => {
    const room = await loadRoom(bus, roomId);
    if (!room) throw new RoomError(ECustomRoomErr.NOROOM);
    if (room.owner !== accountId) throw new RoomError(ECustomRoomErr.NOTOWNER);
    if (room.state !== ROOM_STATE.WAITING) throw new RoomError(ECustomRoomErr.ROOMINGAME);
    // Only the owner-tunable fields; identity (id/owner/members/code) is preserved.
    if (req.room_name) room.name = req.room_name;
    if (req.map_id != null) room.map_id = req.map_id;
    if (req.game_mode != null) room.game_mode = req.game_mode;
    if (req.max_member_num) room.max_member_num = req.max_member_num;
    if (req.max_spectator_num != null) room.max_spectator_num = req.max_spectator_num;
    if (req.enable_death_spectate != null) room.enable_death_spectate = !!req.enable_death_spectate;
    if (req.level_visual_style != null) room.level_visual_style = req.level_visual_style;
    room.room_setting = req.room_setting || 0;
    room.room_setting2 = req.room_setting2 || 0;
    // RoomChangeReq carries no is_cs_advanced field; a non-empty advanced blob implies advanced.
    if (Buffer.isBuffer(req.cs_advanced_setting) && req.cs_advanced_setting.length) {
      room.cs_advanced_b64 = req.cs_advanced_setting.toString('base64');
      room.is_cs_advanced = true;
    }
    await saveRoom(bus, room);
    logger.info(`[room] CHANGE id=${room.id} setting=${room.room_setting} setting2=${room.room_setting2} adv=${room.is_cs_advanced}`);
    return { room };
  });
}

async function setReady(account, ready) {
  const bus = getBus();
  if (!bus) throw new RoomError(ECustomRoomErr.NOTINROOM);
  const accountId = Number(account.uid || account.account_id || 0);
  const roomId = Number(await bus.hget(MEMBERS_KEY, String(accountId)) || 0);
  if (!roomId) throw new RoomError(ECustomRoomErr.NOTINROOM);

  return withRoomLock(bus, roomId, async () => {
    const room = await loadRoom(bus, roomId);
    if (!room) throw new RoomError(ECustomRoomErr.NOTINROOM);
    const m = room.members.find((x) => x.account_id === accountId);
    if (!m) throw new RoomError(ECustomRoomErr.NOTINROOM);
    m.ready = !!ready;
    await saveRoom(bus, room);
    logger.info(`[room] SETREADY id=${room.id} uid=${accountId} ready=${m.ready}`);
    return { room };
  });
}

async function get(roomId) {
  const bus = getBus();
  return bus ? loadRoom(bus, Number(roomId)) : null;
}

async function roomIdOf(accountId) {
  const bus = getBus();
  if (!bus) return 0;
  return Number(await bus.hget(MEMBERS_KEY, String(accountId)) || 0);
}

// --- wire helpers: presence-routed broadcasts to a room's members -----------

// Encode `obj` as `typeName` and push it to `accountId` under ROOM/`cmd` via gw.push (routed
// to whichever gateway holds that client). Returns nothing; best-effort.
function pushToAccount(accountId, cmd, typeName, obj) {
  const bus = getBus();
  if (!bus) return;
  const T = lookup(typeName);
  const content = T ? Buffer.from(T.encode(T.fromObject(obj)).finish()) : Buffer.alloc(0);
  bus.publishPS('gw.push', 'GatewayPush', {
    target_account_id: Number(accountId), protocol: EProtocol.ROOM, cmd, content
  }).catch((e) => logger.warn(`[room] push uid=${accountId} cmd=${cmd}: ${e.message}`));
}

// Broadcast one message to every member of `room` except `exceptId` (usually the requester,
// who already got a direct reply on its own socket).
function broadcast(room, exceptId, cmd, typeName, obj) {
  for (const m of room.members) {
    if (m.account_id === Number(exceptId)) continue;
    pushToAccount(m.account_id, cmd, typeName, obj);
  }
}

// --- *_NTF builders (each carries the full room_info so recipients full-refresh) ---
function joinNtf(room, joiner) { return { join_player_list: [playerInfo(joiner)], room_info: toRoomInfo(room) }; }
function leaveNtf(room, leaver) { return { leaver_info: leaver ? playerInfo(leaver) : {}, room_info: toRoomInfo(room) }; }
function kickNtf(room, target) { return { kick_player_info: playerInfo(target), room_info: toRoomInfo(room) }; }
function setReadyNtf(room) { return { room_id: room.id, players: room.members.map(playerInfo) }; }
function dismissNtf(room, leaverId, reason) { return { leaver_id: Number(leaverId), room_id: room.id, reason }; }

// runOp wraps a room op: on a RoomError it puts the ErrCode in MessageNotify.ret and returns
// `onErr` (empty reply); on any other error it logs + returns a generic failure. Keeps the
// handlers to their happy path.
async function runOp(ctx, body, onErr = {}) {
  try {
    return await body();
  } catch (e) {
    if (e instanceof RoomError) { ctx.ret = e.code; return onErr; }
    ctx.logger.error(`[tcp] room op failed: ${e.stack || e}`);
    ctx.ret = ECustomRoomErr.CREATEROOMFAIL;
    return onErr;
  }
}

// handleDisconnect drops a dropped client from its room (LEAVE, or DISMISS if it was the
// owner) and notifies the survivors with reason OFFLINE. Called from the gateway on socket
// close — best-effort, never throws into the socket path.
const DISMISS_OFFLINE = 2; // tcp.ERoom.DismissReason.OFFLINE
async function handleDisconnect(accountId) {
  try {
    const res = await leave(accountId);
    if (!res) return;
    if (res.dismissed) {
      broadcast({ members: res.members }, accountId, ECustomRoom.DISMISS_NTF,
        'tcp.RoomDismissNtf', dismissNtf(res.room, accountId, DISMISS_OFFLINE));
    } else {
      broadcast(res.room, accountId, ECustomRoom.LEAVE_NTF,
        'tcp.RoomLeaveNtf', leaveNtf(res.room, res.leaver));
    }
  } catch (e) {
    logger.warn(`[room] disconnect cleanup uid=${accountId}: ${e.message}`);
  }
}

module.exports = {
  ROOM_STATE,
  create, list, join, leave, kick, change, setReady, get, roomIdOf,
  toRoomInfo, toBasicInfo, playerInfo,
  pushToAccount, broadcast,
  joinNtf, leaveNtf, kickNtf, setReadyNtf, dismissNtf, runOp, handleDisconnect,
  RoomError
};
