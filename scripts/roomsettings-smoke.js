'use strict';

// Smoke test for the room_setting/room_setting2 bit decoder (src/tcp/roomsettings.js). Verifies the
// RE-confirmed extraction math (index = (setting>>start)&mask; flag = setting&mask) against the
// documented bit positions, and that TABLES resolve indices -> values once populated.
//   node scripts/roomsettings-smoke.js

const assert = require('assert');
const rooms = require('../src/tcp/roomsettings');

// Pack one room_setting with several fields, then confirm each decodes back.
// roundNum index 2 (<<25), initCoin index 1 (<<27), playerHP index 3 (<<8),
// UnlimitedAmmo (0x2), NoSkill (0x20), NoPowerGun (0x20000000), HideEnemyCloth (0x40000000).
const rs =
  (2 << 25) | (1 << 27) | (3 << 8) |
  0x2 | 0x20 | 0x20000000 | 0x40000000;
// room_setting2: FriendDmg (0x20), NoHud (0x10), NoAuxAim (0x4000), revive index 4 (<<8),
// fightClubRound index 1 (<<6).
const rs2 = 0x20 | 0x10 | 0x4000 | (4 << 8) | (1 << 6);

const s = rooms.decodeRoomSettings({ room_setting: rs, room_setting2: rs2, is_cs_advanced: false });

// indices
assert.strictEqual(s.indices.roundNum, 2, 'roundNum index (bits 25-26)');
assert.strictEqual(s.indices.initCoin, 1, 'initCoin index (bits 27-28)');
assert.strictEqual(s.indices.playerHP, 3, 'playerHP index (bits 8-10)');
assert.strictEqual(s.indices.revive, 4, 'revive index (rs2 bits 8-10)');
assert.strictEqual(s.indices.fightClubRound, 1, 'fightClubRound index (rs2 bits 6-7)');
assert.strictEqual(s.indices.playerEP, 0, 'playerEP unset -> 0');

// flags (room_setting)
assert.strictEqual(s.flags.unlimitedAmmo, true, 'UnlimitedAmmo 0x2');
assert.strictEqual(s.flags.noSkill, true, 'NoSkill 0x20');
assert.strictEqual(s.flags.noPowerGun, true, 'NoPowerGun 0x20000000 (bit 29)');
assert.strictEqual(s.flags.hideEnemyCloth, true, 'HideEnemyCloth 0x40000000 (bit 30)');
assert.strictEqual(s.flags.noLoadout, false, 'NoLoadout 0x8 unset');
assert.strictEqual(s.flags.noFallingDamage, false, 'NoFallingDamage 0x4 unset');
// flags (room_setting2)
assert.strictEqual(s.flags.friendlyFire, true, 'FriendDmg 0x20');
assert.strictEqual(s.flags.noHud, true, 'NoHud 0x10');
assert.strictEqual(s.flags.noAuxAim, true, 'NoAuxAim 0x4000');
assert.strictEqual(s.flags.noBomb, false, 'NoBomb 0x2 unset');

// values resolve from the CSV tables (keyed by the packed value = CSV Key, 1-based)
assert.strictEqual(s.roundCount, 13, 'roundNum value 2 -> 13 rounds (CSV Key 2)');
assert.strictEqual(s.maxHP, 50, 'playerHP value 3 -> 50 HP (CSV Key 3)');
assert.deepStrictEqual(s.economyTable, [500, 900, 1100, 1700, 2100, 2400, 3000], 'initCoin value 1 -> default economy table (CSV Key 1)');

// an unset field (value 0) -> undefined -> match keeps its own default; flags default false
const sDefault = rooms.decodeRoomSettings({ room_setting: 0, room_setting2: 0 });
assert.strictEqual(sDefault.roundCount, undefined, 'unset roundNum (value 0) -> default');
assert.strictEqual(sDefault.maxHP, undefined, 'unset HP -> default');
assert.strictEqual(sDefault.flags.unlimitedAmmo, false, 'unset flags -> false');

console.log('roomsettings-smoke OK: bit extraction (indices + flags) + CSV table resolution');
