'use strict';

/**
 * Decode a CS custom-room ADVANCED-settings blob (`cs_advanced_setting`) into a match-settings
 * overlay: a per-round base-coin economy table, the shop allow-list (with host-set prices), and the
 * economy EVENT bonuses. This is the advanced counterpart to the simple-mode bit decoder in
 * roomsettings.js — the two are merged in handlers/RoomStart.js (advanced overrides round count +
 * economy and supplies the shop; simple mode still carries HP / speed / jump / flags).
 *
 * WIRE (IDA-verified against the authoritative reader InitCSShopSettingFromBytes @0x319c754 and the
 * writer GenerateADCSSettingBytes @0x31983c0; all little-endian):
 *
 *   [u8  storeCount]
 *   storeCount × { u8 storeKey, u8 priceStep }     // storeKey = RoomCreate_CSShop.csv INDEXID (1..61)
 *                                                  //   realPrice = priceStep × CS_SHOP_COST_INTERVAL (50)
 *   [u8  ecoCount]
 *   ecoCount  × { u8 eventId, u8 stepIndex }       // eventId = RoomCreateCSEco.csv indexid
 *                                                  //   realCoins = Min + stepIndex × Step  (Min/Step from that CSV)
 *   [u8  roundCount]
 *   roundCount × { i32 perRoundBaseCoin }          // RAW coins (the only un-scaled values in the blob)
 *
 * KEY FACT: the per-store u8 key is the CSV *indexid* (1..61), NOT the item DataID — real DataIDs
 * like 1201/1401/2401 exceed a byte, so the client packs the small row index and we translate it
 * back to the DataID via RoomCreate_CSShop.csv (whose `itemid` col is a compound
 * "<dataId>;<ammo>-<count>;<attachments...>"). See memory cs-room-economy.md.
 */

const fs = require('fs');
const path = require('path');
const logger = require('../logger');

// CustomRoomCSShopCostInterval — the price divisor the client packs shop costs by (realPrice =
// priceStep × 50). IDA-verified from GameVarDef::.cctor @0x27973c8 (#0x32). It is a GameVar and can
// in principle be overridden by a shipped config row; 50 is the shipped-binary baseline.
const CS_SHOP_COST_INTERVAL = 50;

// eco eventId (RoomCreate_CSEco.csv indexid) -> the match server's wireEvents JSON key. Only the six
// open=true economy rows (152..157) are events; 151 (CS_PhasePrepareTime, open=false) is the buy
// timer, not a coin bonus, so it is deliberately absent.
const ECO_EVENT_KEY = {
  152: 'win_round',    // CS_GainCashWinning
  153: 'per_kill',     // CS_GainCashKilling
  154: 'loss_round',   // CS_GainCashLosing
  155: 'loss_streak2', // CS_GainCashLosingStreak_2
  156: 'loss_streak3', // CS_GainCashLosingStreak_3
  157: 'first_blood'   // CS_FirstBloodBonus
};

// eventId -> { name, min, step }, loaded from RoomCreate_CSEco.csv. The blob stores each event as a
// step index; realCoins = min + stepIndex × step. With this populated the decoder emits `events` and
// the match server applies the host's overrides (defaults still equal the economy.go consts).
function loadCSEco(file) {
  const map = {};
  let text;
  try { text = fs.readFileSync(file, 'utf8'); }
  catch (e) { logger.warn(`[csadvanced] cannot read ${file}: ${e.message}`); return map; }
  const lines = text.split(/\r?\n/);
  // row 0 = EN header (indexid,varname,comment,key,min,max,default,step,open,eventkey), row 1 = CN.
  for (let i = 2; i < lines.length; i += 1) {
    const c = lines[i].split(',');
    if (c.length < 9) continue;
    const eventId = parseInt(c[0], 10);
    const name = ECO_EVENT_KEY[eventId];
    if (!name) continue;                                    // not one of the six mapped economy events
    if (String(c[8] || '').trim().toLowerCase() !== 'true') continue; // open=false -> not an event
    map[eventId] = { name, min: parseInt(c[4], 10) || 0, step: parseInt(c[7], 10) || 1 };
  }
  return map;
}

const ECO_STEP = loadCSEco(path.join(__dirname, '../../protocol/RoomCreate_CSEco.csv'));

// Items the match server can't yet PLACE (special-reload weapons, a scope sold as an item, and
// deployables/gadgets that need dedicated slots or spawn logic). They are dropped from the shop
// allow-list here — Node is authoritative for the catalogue, so a host who selects one simply
// doesn't see it, and the match server never receives an item it would mis-slot. See the shop-item
// feasibility map (docs / workflow) for why each is hard.
const QUARANTINE = new Set([
  23,   // M79        — launcher ammo 207, single-shot
  37,   // Gatling    — ammo 210, spin-up, 1200 clip
  100,  // Flamethrower — fuel 214, continuous drain
  537,  // Thermal sight — a scope ATTACHMENT, not a loadout item
  38,   // Grappling gun — gadget weapon
  607,  // Anti-mine magnetic field
  1006, // Personal UAV controller
  1202, // T-brick wall (deployable)
  1401, // Landmine (deployable)
  1601, // Fly shoe / jetpack (passive gadget)
  1602, // Double-jump shoe (passive gadget)
  2102, // Mini-sentry turret (deployable + AI)
  2401  // Dummy decoy (deployable)
]);

// --- CSV loaders (run once at module load) ----------------------------------

// index (RoomCreate_CSShop indexid) -> { itemId, ammo, ammoCount, filter, typeTab, limitation, bonus }.
function loadCSShop(file) {
  const map = new Map();
  let text;
  try { text = fs.readFileSync(file, 'utf8'); }
  catch (e) { logger.warn(`[csadvanced] cannot read ${file}: ${e.message}`); return map; }
  const lines = text.split(/\r?\n/);
  // row 0 = EN header (indexid,itemid,comment,filter,price,stack,limitation,bonus,BuyFilter,typeTab,selected,open)
  // row 1 = CN header. Data starts at row 2. No field contains a comma (the compound itemid uses ';'/'-').
  for (let i = 2; i < lines.length; i += 1) {
    const c = lines[i].split(',');
    if (c.length < 10) continue;
    const index = parseInt(c[0], 10);
    if (!Number.isFinite(index)) continue;
    const tok = String(c[1] || '').split(';');            // "<dataId>;<ammo>-<count>;<attach...>"
    const itemId = parseInt(tok[0], 10);
    if (!Number.isFinite(itemId)) continue;
    let ammo = 0;
    let ammoCount = 0;
    if (tok[1] && tok[1].includes('-')) {
      const [a, n] = tok[1].split('-');
      ammo = parseInt(a, 10) || 0;
      ammoCount = parseInt(n, 10) || 0;
    }
    map.set(index, {
      itemId,
      ammo,
      ammoCount,
      filter: parseInt(c[3], 10) || 0,
      typeTab: parseInt(c[9], 10) || 0,
      limitation: parseInt(c[6], 10) || 0,
      bonus: !!String(c[7] || '').trim() && String(c[7]).trim() !== '0'
    });
  }
  return map;
}

// itemId -> ItemTable `type` (col 11): 0 weapon, 1 consumable, 2 helmet, 4 vest, 6 sight, 8 grenade,
// 12 gloo/building. Used to tell vest from helmet, grenade from gloo, consumable from the rest.
function loadItemTypes(file) {
  const map = new Map();
  let text;
  try { text = fs.readFileSync(file, 'utf8'); }
  catch (e) { logger.warn(`[csadvanced] cannot read ${file}: ${e.message}`); return map; }
  const lines = text.split(/\r?\n/);
  for (let i = 2; i < lines.length; i += 1) {           // rows 0/1 are EN/CN headers
    const c = lines[i].split(',');
    if (c.length <= 11) continue;
    const id = parseInt(c[0], 10);
    if (!Number.isFinite(id)) continue;
    map.set(id, parseInt(c[11], 10) || 0);
  }
  return map;
}

const SHOP_BY_INDEX = loadCSShop(path.join(__dirname, '../../protocol/RoomCreate_CSShop.csv'));
const ITEM_TYPE = loadItemTypes(path.join(__dirname, '../../protocol/ItemTable.csv'));

// ItemTable `type` values used below.
const IT_CONSUMABLE = 1;
const IT_HELMET = 2;
const IT_VEST = 4;
const IT_GRENADE = 8;
const ITEM_MUSHROOM = 709;
const ITEM_GLOO = 1201;

/**
 * Resolve the loadout `slotKind` the match server places an item into, from its ammo + shop filter
 * (1 weapon / 2 armour|utility / 4 throwable) + ItemTable type. Returns null for anything the match
 * server can't place (caller drops it). NOTE: typeTab is deliberately NOT used — it is unreliable in
 * this CSV (Sickle=8, several rifles=4, gloo/turret=3).
 */
function resolveSlotKind(itemId, ammo, filter, itemType) {
  if (QUARANTINE.has(itemId)) return null;
  switch (filter) {
    case 1: // weapon
      if (ammo === 202) return 'secondary';                 // pistols equip in the secondary slot
      if (ammo === 201 || ammo === 203 || ammo === 204 || ammo === 205) return 'primary';
      if (ammo === 0) return 'melee';                       // sickle (only non-quarantined ammo-less gun)
      return null;                                          // exotic ammo (207/210/214) — shouldn't reach here (quarantined)
    case 2: // armour / utility
      if (itemType === IT_VEST) return 'vest';
      if (itemType === IT_HELMET) return 'helmet';
      if (itemType === IT_CONSUMABLE) return 'consumable';  // armour repair kit / adrenaline
      return null;
    case 4: // throwable
      if (itemId === ITEM_MUSHROOM) return 'consumable';    // instant-EP path (match server special-cases 709)
      if (itemId === ITEM_GLOO) return 'building';          // gloo wall -> its own Building slot
      if (itemType === IT_GRENADE) return 'grenade';        // frag / flash / smoke
      return null;
    default:
      return null;
  }
}

/**
 * Decode a `cs_advanced_setting` buffer into an overlay, or null when the blob is empty/malformed
 * (the caller then falls back to simple mode — a bad blob never breaks the start handoff).
 *
 * @returns {null | {
 *   roundCount: number,
 *   perRoundBase: number[],                 // -> economy table (table[0] = starting money)
 *   shopItems: Array<{ item_id, price, filter, limitation, bonus, ammo, slot_kind }>,
 *   events: object|undefined,               // undefined until RoomCreateCSEco.csv is captured
 *   dropped: number[]                        // itemIds the host selected but we can't place (for logging)
 * }}
 */
function decodeAdvanced(buf) {
  if (!Buffer.isBuffer(buf) || buf.length < 3) return null;
  let o = 0;
  const u8 = () => { if (o + 1 > buf.length) throw new RangeError('u8 past end'); return buf.readUInt8(o++); };
  const i32 = () => { if (o + 4 > buf.length) throw new RangeError('i32 past end'); const v = buf.readInt32LE(o); o += 4; return v; };
  try {
    // --- store section: allowed shop items + host prices ---
    const storeCount = u8();
    const shopItems = [];
    const dropped = [];
    for (let i = 0; i < storeCount; i += 1) {
      const idx = u8();
      const step = u8();
      const meta = SHOP_BY_INDEX.get(idx);
      if (!meta) { dropped.push(idx); continue; }           // unknown row index -> skip
      const slotKind = resolveSlotKind(meta.itemId, meta.ammo, meta.filter, ITEM_TYPE.get(meta.itemId) || 0);
      if (!slotKind) { dropped.push(meta.itemId); continue; } // quarantined / unplaceable -> drop
      shopItems.push({
        item_id: meta.itemId,
        price: step * CS_SHOP_COST_INTERVAL,
        filter: meta.filter,
        limitation: meta.limitation,
        bonus: meta.bonus,
        ammo: meta.ammo,
        slot_kind: slotKind
      });
    }

    // --- eco section: event bonuses (stored as step indices) ---
    const ecoCount = u8();
    const events = {};
    for (let i = 0; i < ecoCount; i += 1) {
      const evId = u8();
      const step = u8();
      const es = ECO_STEP[evId];
      if (es) events[es.name] = es.min + step * es.step;
    }

    // --- round section: raw per-round base-coin table ---
    // Clamp to >= 0: the values are signed i32, and a negative would serialize to JSON the Go side
    // can't unmarshal into []uint32 — which would make loadMatchSettings bail and discard EVERY
    // setting (shop/HP/flags too), not just the economy. The client UI never emits a negative coin.
    const roundCount = u8();
    const perRoundBase = [];
    for (let i = 0; i < roundCount; i += 1) perRoundBase.push(Math.max(0, i32()));

    return {
      roundCount,
      perRoundBase,
      shopItems,
      events: Object.keys(events).length ? events : undefined,
      dropped
    };
  } catch (e) {
    logger.warn(`[csadvanced] blob decode failed (${e.message}) — falling back to simple mode`);
    return null;
  }
}

module.exports = {
  decodeAdvanced,
  resolveSlotKind,
  CS_SHOP_COST_INTERVAL,
  QUARANTINE,
  SHOP_BY_INDEX,
  ITEM_TYPE,
  ECO_STEP
};
