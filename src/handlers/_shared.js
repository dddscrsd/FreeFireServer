/**
 * Shared handler helpers + constants.
 *
 * Consolidates every common helper that previously lived (and was duplicated,
 * with slight variations) across login.js and ported_*.js. Endpoint modules
 * `require('./_shared')` and use these instead of re-declaring their own.
 *
 * Duplicated helpers were reconciled into a single SUPERSET that preserves every
 * field any caller relied on:
 *   - buildSelectedItems: union of the ported_0 / ported_2 / ported_9 variants
 *     (keeps shows + emotes from ported_0, and the loadouts->{loadout_id} mapping
 *     from ported_2; loadouts/emotes/shows are only emitted when present, so
 *     callers that never set them are unaffected — proto3 drops empty defaults).
 *   - buildAccountInfoBasic (ported_0) and accountToBasic (ported_8) are KEPT as
 *     separate functions: they target the same proto but were tuned differently
 *     (role/external_type derivation), so both are preserved verbatim.
 *   - buildAvatarProfile (ported_9) and mapProfileToAvatarProfile (ported_7) are
 *     likewise preserved separately (different unlocked_level / equiped_skills
 *     handling).
 */

'use strict';

const crypto = require('crypto');
const config = require('../../config/default');

// --- constants -------------------------------------------------------------

const DEFAULT_REGION = 'BR';
const TOKEN_TTL = 28800; // seconds (8h), matches reference MajorLoginRes.ttl

// --- time / id helpers -----------------------------------------------------

function nowSecs() {
  return Math.floor(Date.now() / 1000);
}

function nowMs() {
  return Date.now();
}

function newToken() {
  return crypto.randomBytes(24).toString('hex');
}

function randInt(n) {
  return Math.floor(Math.random() * n);
}

// --- request / transport helpers -------------------------------------------

function serverUrl(ctx) {
  // Explicit override wins (e.g. behind a proxy / fixed public URL).
  if (config.protocol && config.protocol.serverUrl) return config.protocol.serverUrl;
  // Otherwise derive from the host the client connected on, but swap the port to
  // the MAIN server's port. This redirects the client off the login server
  // (3001) onto the main server (3002) after MajorLogin, and is also correct for
  // GetLoginData (which runs on main and points its log URLs at main).
  const rawHost = (ctx.req.headers.host || '').trim();
  if (!rawHost) return '';
  const hostNoPort = rawHost.replace(/:\d+$/, '');
  const mainPort = (config.ports && config.ports.main) || 3002;
  return `http://${hostNoPort}:${mainPort}`;
}

/**
 * requireAccount(ctx): return the authenticated player document, or null when
 * the request had no/invalid Bearer token. The router has already resolved and
 * attached ctx.account; handlers on authed endpoints can rely on it being set,
 * but should still guard defensively (returning {} yields a benign empty res).
 */
function requireAccount(ctx) {
  return ctx && ctx.account ? ctx.account : null;
}

// --- response builders -----------------------------------------------------

/**
 * Build a `SelectedItems` plain object from an account's stored selected_items.
 * Superset of the previous per-file variants:
 *   - scalar + scalar-array fields (avatar_id/skin_color/banner_id/head_pic/
 *     clothes/slots/shows) always emitted,
 *   - loadouts mapped from a flat id array onto repeated tcp.LoadoutInfo
 *     ({loadout_id}) — only when present,
 *   - emotes ({emotes:[...]}) only when present.
 * loadouts/emotes are only included when the store actually holds them, so this
 * never mis-decodes for callers that keep them empty.
 */
function buildSelectedItems(acc) {
  const si = (acc && acc.selected_items) || {};
  const out = {
    avatar_id: si.avatar_id || 0,
    skin_color: si.skin_color || 0,
    banner_id: si.banner_id || 0,
    head_pic: si.head_pic || 0,
    clothes: Array.isArray(si.clothes) ? si.clothes : [],
    slots: Array.isArray(si.slots) ? si.slots : [],
    shows: Array.isArray(si.shows) ? si.shows : []
  };
  if (Array.isArray(si.loadouts) && si.loadouts.length) {
    out.loadouts = si.loadouts.map((id) => ({ loadout_id: id }));
  }
  if (si.emotes && Array.isArray(si.emotes.emotes)) {
    out.emotes = { emotes: si.emotes.emotes };
  }
  return out;
}

/** Build an `AccountInfoBasic` plain object from an account document. (ported_0) */
function buildAccountInfoBasic(acc) {
  const si = acc.selected_items || {};
  const clan = acc.clan || {};
  return {
    account_id: acc.uid || acc.account_id || 0,
    account_type: acc.account_type || 0,
    nickname: acc.nickname || '',
    external_id: acc.open_id || '',
    region: acc.region || DEFAULT_REGION,
    level: acc.level || 1,
    exp: acc.exp || 0,
    external_type: acc.external_type || 0,
    external_name: acc.external_name || '',
    external_icon: acc.external_icon || '',
    banner_id: si.banner_id || 0,
    head_pic: si.head_pic || 0,
    clan_name: clan.name || '',
    rank: acc.rank || 1,
    ranking_points: acc.ranking_points || 0,
    role: acc.role || 0,
    has_elite_pass: !!acc.elite_pass,
    badge_cnt: acc.ep_badge_count || 999,
    badge_id: acc.ep_badge_id || 1001000021,
    season_id: 1,
    liked: 9999,
    is_deleted: false,
    show_rank: true,
    last_login_at: acc.last_login_at ? Math.floor(acc.last_login_at / 1000) : 0,
    external_uid: 0,
    return_at: 0,
    weapon_skin_shows: acc.selected_items && Array.isArray(acc.selected_items.shows) ? acc.selected_items.shows : []
  };
}

/**
 * Build an `AccountInfoBasic` plain object — ported_8 variant (used by clan /
 * search / member endpoints). Differs from buildAccountInfoBasic in how it
 * derives external_type (from open_id_type) and role (from clan role).
 */
function accountToBasic(acc) {
  const si = acc.selected_items || {};
  return {
    account_id: acc.uid,
    account_type: acc.account_type || 0,
    nickname: acc.nickname || '',
    external_id: acc.open_id || '',
    region: acc.region || '',
    level: acc.level || 1,
    exp: acc.exp || 0,
    external_type: acc.open_id_type ? Number(acc.open_id_type) || 0 : 0,
    external_name: '',
    external_icon: '',
    banner_id: si.banner_id || 0,
    head_pic: si.head_pic || 0,
    clan_name: (acc.clan && acc.clan.name) || '',
    rank: acc.rank || 1,
    ranking_points: acc.ranking_points || 0,
    role: (acc.clan && acc.clan.role && acc.clan.role !== 'none' ? 1 : 0),
    has_elite_pass: !!acc.elite_pass,
    badge_cnt: acc.ep_badge_count || 0,
    badge_id: acc.ep_badge_id || 0,
    season_id: 1,
    liked: 0,
    is_deleted: false,
    show_rank: true,
    last_login_at: acc.last_login_at ? Math.floor(acc.last_login_at / 1000) : 0,
    external_uid: 0,
    return_at: 0
  };
}

/**
 * Map an account to AccountInfoWithPresence (ported_8 GetFriend). `now` is the
 * presence update time (unix seconds).
 */
function accountToPresence(acc, now) {
  const si = acc.selected_items || {};
  return {
    account_id: acc.uid,
    account_type: acc.account_type || 1,
    nickname: acc.nickname || '',
    external_id: acc.open_id || '',
    external_name: '',
    external_type: acc.open_id_type ? Number(acc.open_id_type) || 0 : 0,
    region: acc.region || '',
    portrait: String(si.head_pic || 902000011),
    level: acc.level || 1,
    exp: acc.exp || 0,
    update_time: now,
    rank: acc.rank || 1,
    ranking_points: acc.ranking_points || 0,
    banner_id: si.banner_id || 901000036,
    head_pic: si.head_pic || 902000011,
    clan_name: (acc.clan && acc.clan.name) || '',
    has_elite_pass: !!acc.elite_pass,
    badge_cnt: acc.ep_badge_count || 0,
    badge_id: acc.ep_badge_id || 0,
    is_deleted: false,
    show_rank: true,
    last_login_at: acc.last_login_at ? Math.floor(acc.last_login_at / 1000) : 0,
    external_uid: 0
  };
}

/** Return the selected profile (or the first) from an account document. */
function selectedProfile(acc) {
  const profiles = (acc.profile && acc.profile.profiles) || [];
  return profiles.find((p) => p && p.is_selected) || profiles[0] || null;
}

/** Map one stored profile doc to the proto AvatarProfile shape. (ported_9) */
function buildAvatarProfile(p) {
  return {
    avatar_id: p.avatar_id || 0,
    unlocked_level: p.unlocked_level || 1,
    skin_color: p.skin_color || 0,
    clothes: Array.isArray(p.clothes) ? p.clothes : [],
    equiped_skills: (Array.isArray(p.equiped_skills) ? p.equiped_skills : []).map((s) => ({
      slot_id: s.slot_id || 0,
      skill_id: s.skill_id || 0
    })),
    is_selected: !!p.is_selected
  };
}

/**
 * Map one stored profile doc to AvatarProfile — ported_7 variant (ChangeClothes).
 * Differs from buildAvatarProfile: unlocked_level defaults to 0 and
 * equiped_skills is passed through unmapped (unknown keys ignored by fromObject).
 */
function mapProfileToAvatarProfile(p) {
  return {
    avatar_id: p.avatar_id || 0,
    unlocked_level: p.unlocked_level || 0,
    skin_color: p.skin_color || 0,
    clothes: Array.isArray(p.clothes) ? p.clothes : [],
    equiped_skills: Array.isArray(p.equiped_skills) ? p.equiped_skills : [],
    is_selected: !!p.is_selected
  };
}

/**
 * Port of sync_selected_items_from_profile (htpp.py:842) — ported_6. Copies the
 * profile's avatar/skin/clothes onto selected_items and rebuilds the 16-slot
 * skill layout from the profile's equipped skills.
 */
function syncSelectedItemsFromProfile(account, profile) {
  if (!profile) return;
  if (!account.selected_items) account.selected_items = {};
  const selected = account.selected_items;
  selected.avatar_id = profile.avatar_id || 0;
  selected.skin_color = profile.skin_color || 0;
  selected.clothes = profile.clothes || [];

  const slots = selected.slots && selected.slots.length ? selected.slots : new Array(16).fill(0);
  for (const skill of profile.equiped_skills || []) {
    const slotId = skill.slot_id || 0;
    if (slotId >= 0 && slotId < slots.length) slots[slotId] = skill.skill_id || 0;
  }
  selected.slots = slots;
}

module.exports = {
  // constants
  DEFAULT_REGION,
  TOKEN_TTL,
  // time / id
  nowSecs,
  nowMs,
  newToken,
  randInt,
  // request / transport
  serverUrl,
  requireAccount,
  // response builders
  buildSelectedItems,
  buildAccountInfoBasic,
  accountToBasic,
  accountToPresence,
  selectedProfile,
  buildAvatarProfile,
  mapProfileToAvatarProfile,
  syncSelectedItemsFromProfile
};
