/**
 * Player-state store.
 *
 * Mirrors the reference (1.43 Flask) MongoDB account document, but persists it
 * as a single JSON `state` column on the existing sqlite `accounts` row (see
 * src/db/accounts.js). A handful of columns (account_id, open_id, nickname,
 * region, level, clan_id, token, token_created_at) are denormalised out of the
 * JSON purely so they can be indexed/queried; `state` is the source of truth
 * for everything game-related.
 *
 * The "account" object handed to handlers is the rich player document:
 *   {
 *     uid, account_id, open_id, open_id_type, nickname, device_id,
 *     client_version, token, token_created_at, account_type,
 *     region, language,
 *     level, exp, rank, ranking_points, coins, gems,
 *     skateboard_id, elite_pass, ep_badge_count, ep_badge_id, game_server_id,
 *     wallet: { coins, gems },
 *     selected_items: { avatar_id, skin_color, clothes[], banner_id, head_pic,
 *                       loadouts[], slots[] },
 *     profile: { profiles: [ { avatar_id, clothes[], equiped_skills[],
 *                              is_selected, skin_color, unlocked_level } ] },
 *     backpack: { items: [] },
 *     friends: [], requests: [], mails: [],
 *     clan: { id, name, role, rank, points, members: [] },
 *     created_at, last_login_at
 *   }
 *
 * uid === account_id (the sqlite AUTOINCREMENT primary key). Both names are
 * populated so handlers can use either (the reference uses `uid`).
 */

'use strict';

const crypto = require('crypto');

const accounts = require('./accounts');
const logger = require('../logger');

const db = accounts.db;

const DEFAULT_REGION = ''; // empty => client prompts for region selection

// ---------------------------------------------------------------------------
// DEFAULT_PLAYER template
//
// Faithful port of the reference account document created on PlatformRegister
// (htpp.py ~line 1937): sensible, non-empty defaults so the client boots
// straight into the lobby with a usable roster, wallet and selected items.
// Returned via a factory so every new account gets its own deep copy (no
// shared array/object references between accounts).
// ---------------------------------------------------------------------------
function DEFAULT_PLAYER() {
  return {
    uid: 0,
    account_id: 0,
    open_id: null,
    open_id_type: null,
    nickname: 'Player',
    device_id: null,
    client_version: null,
    token: null,
    token_created_at: 0,

    banned: false,
    account_type: 0,
    region: null,
    language: null,

    level: 100,
    exp: 250000,
    rank: 1,
    ranking_points: 1000,
    coins: 999999,
    gems: 999999,
    role: 1,

    skateboard_id: 0,
    elite_pass: false,
    ep_badge_count: 0,
    ep_badge_id: 0,
    game_server_id: '',

    // Keep wallet in sync with the headline coins/gems above so the lobby
    // currency (GetLoginData) matches the wallet/backpack (GetWallet/GetBackpack).
    wallet: { coins: 999999, gems: 999999 },

    profile: {
      profiles: [
        {
          avatar_id: 102000004,
          clothes: [203000001, 211000000, 204000001, 205000001],
          equiped_skills: [{skill_id: 102000004, slot_id: 0}],
          is_selected: true,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000001,
          clothes: [203000166],
          equiped_skills: [{skill_id: 101000001, slot_id: 0}],
          is_selected: false,
          skin_color: 38,
          unlocked_level: 1
        },
        {
          avatar_id: 102000005,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 406, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000005,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 106, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000006,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 306, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000006,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 206, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000007,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 706, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000007,
          clothes: [203000092],
          equiped_skills: [{skill_id: 506, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000008,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 806, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000008,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 906, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000009,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 1006, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000009,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 1106, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000010,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 1306, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000010,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 1106, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000011,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 1206, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000011,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 1406, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000012,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 1506, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000012,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 1706, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000013,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 1806, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000013,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 1906, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000014,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 2006, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000014,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 2106, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000015,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 2206, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000015,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 2506, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000016,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 2306, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000016,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 2406, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000017,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 2706, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000017,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 2606, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000018,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 2906, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000018,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 2806, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000019,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 3006, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000019,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 3106, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000020,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 3206, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000020,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 3506, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000021,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 3306, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000021,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 3906, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000022,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 3406, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000022,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 4406, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000023,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 3606, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 101000023,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 4806, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000024,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 3806, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000025,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 4006, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000026,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 4106, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000027,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 4206, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000028,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 4306, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000029,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 4506, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000030,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 4606, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000031,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 4706, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000032,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 4906, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000033,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 5006, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        },
        {
          avatar_id: 102000034,
          clothes: [203000000, 204000000, 200000000, 200000000],
          equiped_skills: [{skill_id: 5206, slot_id: 0}],
          is_selected: false,
          skin_color: 5,
          unlocked_level: 1
        }
      ]
    },

    // selected_items: avatar_id/skin_color/clothes are synced from the selected
    // profile on demand (reference: sync_selected_items_from_profile). We seed
    // the selected profile's values so the lobby renders immediately.
    selected_items: {
      avatar_id: 102000004,
      skin_color: 5,
      clothes: [203000001, 211000000, 204000001, 205000001],
      banner_id: 900000014,
      head_pic: 902000003,
      loadouts: [102000004, 102000005],
      slots: [102000004, 1, 2, 3, 907000010, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    },

    backpack: { items: [] },
    friends: [],
    requests: [],
    mails: [],
    clan: { id: 0, name: null, role: 'none', rank: 0, points: 0, members: [] },

    created_at: 0,
    last_login_at: 0
  };
}

// ---------------------------------------------------------------------------
// prepared statements
// ---------------------------------------------------------------------------
const selByTokenStmt = db.prepare('SELECT * FROM accounts WHERE token = ?');
const selByIdStmt = db.prepare('SELECT * FROM accounts WHERE account_id = ?');
const selByOpenIdStmt = db.prepare('SELECT * FROM accounts WHERE open_id = ?');

const insertStmt = db.prepare(`
  INSERT INTO accounts
    (open_id, open_id_type, nickname, region, language, device_id,
     client_version, level, clan_id, created_at, last_login_at,
     token, token_created_at, raw_login, state)
  VALUES
    (@open_id, @open_id_type, @nickname, @region, @language, @device_id,
     @client_version, @level, @clan_id, @created_at, @last_login_at,
     @token, @token_created_at, @raw_login, @state)
`);

const updateStmt = db.prepare(`
  UPDATE accounts SET
    open_id_type     = @open_id_type,
    nickname         = @nickname,
    region           = @region,
    language         = @language,
    device_id        = @device_id,
    client_version   = @client_version,
    level            = @level,
    clan_id          = @clan_id,
    last_login_at    = @last_login_at,
    token            = @token,
    token_created_at = @token_created_at,
    state            = @state
  WHERE account_id = @account_id
`);

// ---------------------------------------------------------------------------
// row <-> account mapping
// ---------------------------------------------------------------------------
// The platform open_id is THE account key. It is present in both LoginReq
// (field 22) and PlatformRegisterReq (field 3), so register and the subsequent
// login resolve to the same account. The client (UIModelLogin.RequestLogin /
// CreateLoginReqInfoData) only fills open_id/login_token when a real Garena SDK
// login happened; on a guest / no-SDK client they arrive EMPTY, and the request
// carries NO per-device identity shared with the register request.
//
// In that case fall back to a single DETERMINISTIC guest id (configurable via
// GUEST_OPEN_ID) — never a random one. A random fallback was the bug: each call
// derived a different open_id, so the account created at MajorRegister could
// never be found at the next MajorLogin. With a fixed guest id, register and
// login agree. (For multiple distinct accounts the client must supply a real,
// non-empty open_id, e.g. via the Garena SDK.)
const GUEST_OPEN_ID = process.env.GUEST_OPEN_ID || 'guest-default';

function deriveOpenId(obj) {
  obj = obj || {};
  const oid = obj.open_id;
  if (oid != null && String(oid).trim() !== '') return String(oid);
  return GUEST_OPEN_ID;
}

/**
 * Turn a sqlite row into the rich player document. `state` JSON is the source
 * of truth; the indexed columns (PK, token) are overlaid so they stay
 * authoritative even if the JSON drifts.
 */
function rowToAccount(row) {
  if (!row) return null;
  let acc;
  try {
    acc = row.state ? JSON.parse(row.state) : DEFAULT_PLAYER();
  } catch (err) {
    logger.warn(`[player] corrupt state for account_id=${row.account_id}: ${err.message}`);
    acc = DEFAULT_PLAYER();
  }
  acc.uid = row.account_id;
  acc.account_id = row.account_id;
  if (row.open_id != null) acc.open_id = row.open_id;
  if (row.token != null) acc.token = row.token;
  if (row.token_created_at != null) acc.token_created_at = row.token_created_at;
  return acc;
}

/**
 * Persist a player document. Serialises the whole doc into `state` and syncs
 * the denormalised index columns. Returns the account (unchanged).
 */
function save(account) {
  if (!account) return account;
  const accountId = account.uid || account.account_id;
  if (!accountId) {
    throw new Error('player.save: account has no uid/account_id');
  }
  account.uid = accountId;
  account.account_id = accountId;
  account.last_login_at = account.last_login_at || Date.now();

  updateStmt.run({
    account_id: accountId,
    open_id: account.open_id ?? null,
    open_id_type: account.open_id_type ?? null,
    nickname: account.nickname ?? null,
    region: account.region ?? null,
    language: account.language ?? null,
    device_id: account.device_id ?? null,
    client_version: account.client_version ?? null,
    level: account.level ?? 1,
    clan_id: (account.clan && account.clan.id) || account.clan_id || 0,
    last_login_at: account.last_login_at,
    token: account.token ?? null,
    token_created_at: account.token_created_at ?? 0,
    state: JSON.stringify(account)
  });
  return account;
}

// ---------------------------------------------------------------------------
// public API
// ---------------------------------------------------------------------------

/** Resolve a player by its issued login token (Bearer). */
function getByToken(token) {
  if (!token) return null;
  return rowToAccount(selByTokenStmt.get(token));
}

/** Resolve a player by uid / account_id. */
function getById(uid) {
  if (!uid) return null;
  return rowToAccount(selByIdStmt.get(uid));
}

/** Resolve a player by platform open_id. */
function getByOpenId(openId) {
  if (!openId) return null;
  return rowToAccount(selByOpenIdStmt.get(openId));
}

/**
 * Create (or fetch) a player from a decoded login/register request object.
 *
 * If an account already exists for the request's open_id, its identity fields
 * are refreshed from the request and it is returned. Otherwise a fresh account
 * is minted from DEFAULT_PLAYER with identity seeded from the request.
 *
 * @param {object} loginReqObj decoded LoginReq / PlatformRegisterReq plain object
 * @returns {object} the rich player document (with a real uid)
 */
function createFromLogin(loginReqObj) {
  const req = loginReqObj || {};
  const openId = deriveOpenId(req);
  const now = Date.now();

  const existing = selByOpenIdStmt.get(openId);
  if (existing) {
    const acc = rowToAccount(existing);
    // Refresh identity from the incoming request (keep stored values otherwise).
    if (req.open_id_type != null) acc.open_id_type = String(req.open_id_type);
    if (req.nickname) acc.nickname = String(req.nickname);
    if (req.region) acc.region = String(req.region);
    if (req.language) acc.language = String(req.language);
    if (req.device_id) acc.device_id = String(req.device_id);
    if (req.client_version) acc.client_version = String(req.client_version);
    return save(acc);
  }

  // Insert a placeholder row to obtain the AUTOINCREMENT account_id (uid),
  // then build + persist the full default document keyed by that uid.
  const info = insertStmt.run({
    open_id: openId,
    open_id_type: req.open_id_type != null ? String(req.open_id_type) : null,
    nickname: req.nickname ? String(req.nickname) : 'Player',
    region: req.region ? String(req.region) : null,
    language: req.language ? String(req.language) : null,
    device_id: req.device_id ? String(req.device_id) : null,
    client_version: req.client_version ? String(req.client_version) : null,
    level: 100,
    clan_id: 0,
    created_at: now,
    last_login_at: now,
    token: null,
    token_created_at: 0,
    raw_login: JSON.stringify(req),
    state: '{}'
  });

  const acc = DEFAULT_PLAYER();
  acc.uid = info.lastInsertRowid;
  acc.account_id = info.lastInsertRowid;
  acc.open_id = openId;
  acc.open_id_type = req.open_id_type != null ? String(req.open_id_type) : null;
  if (req.nickname) acc.nickname = String(req.nickname);
  acc.region = req.region ? String(req.region) : null;
  acc.language = req.language ? String(req.language) : null;
  acc.device_id = req.device_id ? String(req.device_id) : null;
  acc.client_version = req.client_version ? String(req.client_version) : null;
  acc.created_at = now;
  acc.last_login_at = now;

  logger.info(`[player] created account uid=${acc.uid} open_id=${openId}`);
  return save(acc);
}

module.exports = {
  DEFAULT_PLAYER,
  DEFAULT_REGION,
  deriveOpenId,
  getByToken,
  getById,
  getByOpenId,
  createFromLogin,
  save
};
