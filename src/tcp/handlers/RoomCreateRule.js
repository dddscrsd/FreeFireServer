'use strict';

// ROOM / ROOM_CREATE_RULE (subcmd 32). The client sends this with an EMPTY body when the room
// center / room list opens (COW::UIModelCustomRoom::RequestRoomCreateRules). The response's two
// lists set IsRoomCreateRulesInited + IsRoomCreateRuleDropInited, which gate
// UIRoomListController::OnBtnRoomCreate — WITHOUT this reply the Create button does nothing for
// Normal(room_type=1) / League(room_type=2) modes (Pet Rumble tab5 / Pet Mania tab3 bypass the
// gate with client-baked rules, which is why only those worked). See docs/custom-room-design.md.
//
// The create dialog is FULLY server-driven: the mode list (GetModeIDListByRoomType), the map
// grid (GetMapIDListByGameModeAndRoomType), members/spectators and the group toggles are ALL
// built from this response — the client has no independent whitelist. So any self-consistent
// ids populate the dialog; real ids only matter for correct on-screen NAMES. We advertise Clash
// Squad: game_mode 15 (the client's squad-first CS mode) on map_id 11 = "Fight Club", the CS
// arena whose name is baked in GetMapNameByMapId (map 1 is Paradise, a BR map). mapConfigId =
// map_id*1000 + game_mode = 11015.
const { EProtocol, ECustomRoom } = require('../protocol');

const CS_MAP = 11;         // Fight Club — the CS arena (baked TXT map name); mapConfigId 11015
const CS_MODE = 15;        // game_mode 15: client special-cases this as squad-first (Clash Squad)
const GROUP_SQUAD = 3;     // group_mode: 0 Solo, 1 Duo, 3 Squad/Quad, 5 Hexa
const ROOM_TYPE_CASUAL = 1;
const ROOM_TYPE_LEAGUE = 2;

// One CS create rule for the given room_type (Normal / League). members[0] is the default; we
// offer 4v4 down to 1v1 (total counts) so the host can pick a team size.
function csRule(roomType) {
  return {
    map_id: CS_MAP,
    game_mode: CS_MODE,
    group_mode: GROUP_SQUAD,
    members: [8, 6, 4, 2],
    spectators: [0, 1, 2],
    room_type: roomType,
    min_member_cnt: 2,
    enable_voice_chat: true
  };
}

// A minimal CS drop/preset so IsRoomCreateRuleDropInited flips and the CS settings panel has a
// baseline. drop_type=1 routes it into the client's CS drop list. loc_key/describe_key are the
// preset's localization keys (label only — a wrong key mislabels the preset, it doesn't block).
function csDrop(id, roomType) {
  return {
    id,
    room_type: roomType,
    drop_type: 1,
    loc_key: 'RoomCreateDrop_CS',
    describe_key: 'RoomCreateDropDesc_CS',
    unlimited_ammo_switch: 0,
    air_drop_switch: 0,
    loadout_switch: 1,
    car_drop_switch: 0,
    air_ship_switch: 0,
    ban_gun_skin_attr: 0,
    hide_enemy_fashion: 0,
    friendly_fire: 0,
    hide_hud: 0,
    revival: 0
  };
}

async function handler(reqObj, ctx) {
  const res = {
    room_create_rules: [csRule(ROOM_TYPE_CASUAL), csRule(ROOM_TYPE_LEAGUE)],
    room_create_rule_drops: [csDrop(1, ROOM_TYPE_CASUAL), csDrop(2, ROOM_TYPE_LEAGUE)]
  };
  ctx.logger.info(`[tcp] RoomCreateRule uid=${ctx.account.uid} -> ${res.room_create_rules.length} rules / ${res.room_create_rule_drops.length} drops`);
  return res;
}

module.exports = {
  protocol: EProtocol.ROOM,               // 14
  subcmd: ECustomRoom.ROOM_CREATE_RULE,   // 32
  // proto.RoomCreateRuleRes (package `proto`, from gen_proto.py) has the AUTHORITATIVE tag
  // numbers — the DropDesc switch fields are DropPresetState enums at non-sequential tags
  // (6/8/10/11/12/13/14/15/16/17/18/19), so hand-numbering them collides a string onto an enum
  // tag and the client's protobuf-net throws Invalid wire-type. Always use the generated type.
  resType: 'proto.RoomCreateRuleRes',
  handler
};
