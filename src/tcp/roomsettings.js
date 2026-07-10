'use strict';

/**
 * Decode the simple-mode custom-room settings packed into RoomInfo.room_setting / room_setting2.
 *
 * Extraction math RE-confirmed (GetRoomSettingValue @0x319d1b0 / GenerateCustomRoomGameSettingDict
 * @0x31a57b0):
 *   multi-bit field -> INDEX  = (setting >>> startBit) & ((1 << width) - 1)   // NOT the raw value
 *   single-bit flag        = (setting & mask) !== 0
 *
 * The multi-bit fields store an INDEX; the index -> value tables (round count, HP, start coins,
 * revive rule) live in the CLIENT-baked CSV CONFIG_ROOM_CREATE_RULE_HPEP (type-demuxed 0-7) — NOT
 * in the binary and NOT shipped by the server. Populate TABLES below from the extracted CSV to
 * resolve the actual values. Until then decodeRoomSettings still returns the raw indices + flags.
 * See memory cs-room-economy.md.
 */

// Multi-bit index fields in room_setting: name -> [startBit, width].
const IDX = {
  playerHP: [8, 3],     // bits 8-10  (index 0-7)
  playerEP: [11, 3],    // bits 11-13
  playerSpeed: [14, 3], // bits 14-16
  dropList: [17, 4],    // bits 17-20 (BR)
  playerJump: [21, 3],  // bits 21-23
  roundNum: [25, 2],    // bits 25-26 (index 0-3) — the CS ROUND COUNT
  initCoin: [27, 2]     // bits 27-28 (index 0-3) — CS start money
};
// Multi-bit index fields in room_setting2.
const IDX2 = {
  fightClubRound: [6, 2], // bits 6-7 (Fight Club sub-mode round count — separate from roundNum)
  revive: [8, 3]          // bits 8-10 (revive/respawn rule; value is a loc key, not a number)
};
// Single-bit flags in room_setting.
const FLAG = {
  hideKillInfo: 0x1, unlimitedAmmo: 0x2, noFallingDamage: 0x4, noLoadout: 0x8,
  noAirdrop: 0x10, noSkill: 0x20, noVehicle: 0x40, accTotalStats: 0x1000000,
  noPowerGun: 0x20000000, hideEnemyCloth: 0x40000000
};
// Single-bit flags in room_setting2.
const FLAG2 = {
  noUav: 0x1, noBomb: 0x2, replay: 0x4, noZeppelin: 0x8, noHud: 0x10,
  friendlyFire: 0x20, inGameChat: 0x800, shopFlow: 0x1000, useRandomMap: 0x2000, noAuxAim: 0x4000
};

// value -> setting tables from CONFIG_ROOM_CREATE_RULE_HPEP. KEYED BY THE PACKED VALUE, which is the
// CSV Key (1-based); value 0 = unset -> default (undefined). The 2-bit fields (roundNum/initCoin)
// only reach Keys 1-3 / 1-2 in simple mode; wider Keys need advanced mode.
const TABLES = {
  roundNum: { 1: 7, 2: 13, 3: 11, 4: 5, 5: 3 },                       // Type 4: CS round count
  initCoin: {                                                          // Type 5: per-round base ECONOMY preset (';'-sep in CSV)
    1: [500, 900, 1100, 1700, 2100, 2400, 3000],
    2: [1500, 1700, 1900, 2100, 2300, 2500, 2700]
  },
  playerHP: { 1: 200, 2: 500, 3: 50, 4: 1 },                          // Type 0
  playerEP: { 1: 0, 2: 50, 3: 200 },                                  // Type 1
  playerSpeed: { 1: 1.0, 2: 0.5, 3: 1.25, 4: 2.0 },                   // Type 2 (move-speed multiplier)
  playerJump: { 1: 1.0, 2: 2.0, 3: 4.0 },                             // Type 3 (jump-height multiplier)
  fightClubRound: { 1: 9, 2: 11, 3: 13 },                             // Type 6
  revive: { 1: '1;1', 2: '1;0', 3: '0;1', 4: '0;0' }                  // Type 7: revivePoint;reviveCard flags
};

const idxOf = (v, [start, width]) => ((v >>> start) & ((1 << width) - 1));
const hasFlag = (v, mask) => (v & mask) !== 0;
// table is KEYED BY THE PACKED VALUE (= CSV Key); value 0 (unset) -> undefined -> match keeps default.
const resolve = (table, v) => (v ? table[v] : undefined);

function decodeIndices(rs, rs2) {
  const out = {};
  for (const [k, spec] of Object.entries(IDX)) out[k] = idxOf(rs, spec);
  for (const [k, spec] of Object.entries(IDX2)) out[k] = idxOf(rs2, spec);
  return out;
}

function decodeFlags(rs, rs2) {
  const out = {};
  for (const [k, m] of Object.entries(FLAG)) out[k] = hasFlag(rs, m);
  for (const [k, m] of Object.entries(FLAG2)) out[k] = hasFlag(rs2, m);
  return out;
}

/**
 * Decode a room record into normalized settings. `flags` + `indices` are always populated (the
 * extraction math is exact); the resolved VALUES (roundCount/maxHP/initCoin) require TABLES to be
 * filled from the CSV — undefined until then, so the match-server keeps its defaults.
 */
function decodeRoomSettings(room) {
  const rs = (room && room.room_setting) || 0;
  const rs2 = (room && room.room_setting2) || 0;
  const indices = decodeIndices(rs, rs2);
  return {
    flags: decodeFlags(rs, rs2),
    indices,
    roundCount: resolve(TABLES.roundNum, indices.roundNum),    // CS round count (undefined = default)
    maxHP: resolve(TABLES.playerHP, indices.playerHP),         // HP (undefined = default)
    maxEP: resolve(TABLES.playerEP, indices.playerEP),
    speedMul: resolve(TABLES.playerSpeed, indices.playerSpeed),
    jumpMul: resolve(TABLES.playerJump, indices.playerJump),
    economyTable: resolve(TABLES.initCoin, indices.initCoin),  // per-round base coin table (undefined = default)
    revive: resolve(TABLES.revive, indices.revive),
    isCsAdvanced: !!(room && room.is_cs_advanced)
  };
}

module.exports = { decodeRoomSettings, decodeIndices, decodeFlags, idxOf, hasFlag, IDX, IDX2, FLAG, FLAG2, TABLES };
