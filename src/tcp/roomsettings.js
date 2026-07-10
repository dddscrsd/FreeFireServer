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

// index -> value tables from CONFIG_ROOM_CREATE_RULE_HPEP. FILL from the extracted CSV (arrays
// indexed by the packed index). Empty => value resolves to undefined (caller falls back to default).
const TABLES = {
  roundNum: [],  // index -> CS round count (e.g. [7, 9, 13, ...])
  playerHP: [],  // index -> HP
  initCoin: [],  // index -> starting coins
  revive: []     // index -> revive-rule id/loc
};

const idxOf = (v, [start, width]) => ((v >>> start) & ((1 << width) - 1));
const hasFlag = (v, mask) => (v & mask) !== 0;
const resolve = (table, i) => (Array.isArray(table) && i >= 0 && i < table.length ? table[i] : undefined);

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
    roundCount: resolve(TABLES.roundNum, indices.roundNum),
    maxHP: resolve(TABLES.playerHP, indices.playerHP),
    initCoin: resolve(TABLES.initCoin, indices.initCoin),
    revive: resolve(TABLES.revive, indices.revive),
    isCsAdvanced: !!(room && room.is_cs_advanced)
  };
}

module.exports = { decodeRoomSettings, decodeIndices, decodeFlags, idxOf, hasFlag, IDX, IDX2, FLAG, FLAG2, TABLES };
