'use strict';

// Admin command core — frontend-agnostic. Every command is an async method that
// returns a plain result object (no terminal formatting), so the CLI
// (scripts/admin.js) and a future Discord bot can share it verbatim.
//
// It is a THIN client over infra that already exists:
//   - player data  -> getRepo() (src/db/repo)         getById / save / getByIds / searchByNickname
//   - kick / cache -> Bus (src/bus)                   publishPS session.revoke / player.updated
//   - presence     -> Bus.scanPresence / getNode
//   - fleet/queue  -> Bus.getMatchServers / hlen('mmqueue')
//   - auth ban flag + audit -> its own small pg Pool  (config.postgres.url)
//
// Every MUTATING command writes an admin_audit row. All external calls are
// best-effort + logged: a missing Redis/Postgres degrades a command, never throws
// out of the process.

const os = require('os');
const config = require('../../config/default');
const logger = require('../logger');
const { getRepo } = require('../db/repo');
const authStore = require('../db/authStore');
const { Bus } = require('../bus');

function safeSchema() {
  const s = (config.auth && config.auth.pgSchema) || 'auth';
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(s) ? s : 'auth';
}

class AdminCore {
  constructor({ actor } = {}) {
    this.actor = actor || process.env.ADMIN_ACTOR || (os.userInfo && os.userInfo().username) || 'cli';
    this.schema = safeSchema();
    this.repo = getRepo();
    this.bus = (config.redis && config.redis.url)
      ? new Bus({ url: config.redis.url, source: 'admin', node: 'admin' })
      : null;
    // Own pool for the auth ban flag + the audit log. Lazy require so a SQLite-only
    // dev box (no DATABASE_URL) doesn't need pg for the read-only commands.
    this.pool = null;
    if (config.postgres && config.postgres.url) {
      const { Pool } = require('pg');
      this.pool = new Pool({ connectionString: config.postgres.url, max: 3 });
      this.pool.on('error', (e) => logger.error(`[admin] pool: ${e.message}`));
    }
    this._auditReady = false;
  }

  // --- read commands --------------------------------------------------------

  async whois(id) {
    const uid = Number(id);
    if (!uid) return { found: false, error: 'invalid id' };
    const acc = await this.repo.getById(uid);
    if (!acc) return { found: false, uid };
    const node = this.bus ? await this.bus.getNode(uid).catch(() => null) : null;
    let auth = null;
    if (acc.open_id) {
      try {
        const a = await authStore.accountByOpenId(acc.open_id);
        if (a) auth = { banned: a.banned === true, authorized: a.authorized !== false, ban_reason: a.ban_reason };
      } catch (e) { /* best-effort */ }
    }
    return {
      found: true, uid, nickname: acc.nickname, open_id: acc.open_id,
      role: acc.role, level: acc.level,
      banned: acc.banned === true || !!(auth && (auth.banned || !auth.authorized)),
      online: !!node, node, auth,
    };
  }

  async find(name) {
    const ids = await this.repo.searchByNickname(String(name || ''));
    const accts = ids && ids.length ? await this.repo.getByIds(ids) : [];
    return accts.map((a) => ({ uid: a.uid, nickname: a.nickname, level: a.level }));
  }

  async online() {
    if (!this.bus) return [];
    const map = await this.bus.scanPresence().catch(() => ({}));
    return Object.entries(map).map(([uid, node]) => ({ uid: Number(uid), node })).sort((a, b) => a.uid - b.uid);
  }

  async stats() {
    const presence = this.bus ? await this.bus.scanPresence().catch(() => ({})) : {};
    const queue = this.bus ? await this.bus.hlen('mmqueue').catch(() => 0) : 0;
    const fleet = this.bus ? await this.bus.getMatchServers().catch(() => []) : [];
    const redis = this.bus ? await this.bus.pub.ping().then(() => true).catch(() => false) : false;
    const postgres = this.pool ? await this.pool.query('SELECT 1').then(() => true).catch(() => false) : false;
    return { online: Object.keys(presence).length, queue: Number(queue) || 0, matchServers: fleet, redis, postgres };
  }

  // --- mutating commands ----------------------------------------------------

  async setNickname(id, name) {
    const uid = Number(id);
    if (!uid) return { ok: false, error: 'invalid id' };
    if (!name || !String(name).trim()) return { ok: false, error: 'empty nickname' };
    const acc = await this.repo.getById(uid);
    if (!acc) return { ok: false, uid, error: 'not found' };
    const from = acc.nickname;
    acc.nickname = String(name);
    await this.repo.save(acc);
    this._publishUpdated(uid);
    await this.audit('nickname', uid, { from, to: acc.nickname });
    return { ok: true, uid, from, to: acc.nickname };
  }

  async setRole(id, role) {
    const uid = Number(id);
    const r = Number(role);
    if (!uid) return { ok: false, error: 'invalid id' };
    if (!Number.isInteger(r)) return { ok: false, error: 'role must be an integer' };
    const acc = await this.repo.getById(uid);
    if (!acc) return { ok: false, uid, error: 'not found' };
    const from = acc.role;
    acc.role = r;
    await this.repo.save(acc);
    this._publishUpdated(uid);
    await this.audit('role', uid, { from, to: r });
    return { ok: true, uid, from, to: r };
  }

  async kick(id, reason) {
    const uid = Number(id);
    if (!uid) return { ok: false, error: 'invalid id' };
    const res = await this._revoke(uid, reason || 'kicked by admin');
    await this.audit('kick', uid, { reason: reason || null });
    return { ok: true, uid, ...res };
  }

  async ban(id, reason) {
    const uid = Number(id);
    if (!uid) return { ok: false, error: 'invalid id' };
    const acc = await this.repo.getById(uid);
    let gameFlag = false;
    if (acc) {
      acc.banned = true;
      acc.ban_reason = String(reason || 'banned by admin');
      await this.repo.save(acc);
      gameFlag = true;
    }
    const authFlag = await this._setAuthBan(acc && acc.open_id, uid, true, reason || 'banned by admin');
    const revoke = await this._revoke(uid, reason || 'banned by admin');
    await this.audit('ban', uid, { reason: reason || null, gameFlag, authFlag });
    return { ok: gameFlag || authFlag.ok, uid, gameFlag, authFlag, ...revoke };
  }

  async unban(id) {
    const uid = Number(id);
    if (!uid) return { ok: false, error: 'invalid id' };
    const acc = await this.repo.getById(uid);
    let gameFlag = false;
    if (acc && acc.banned) {
      acc.banned = false;
      acc.ban_reason = null;
      await this.repo.save(acc);
      this._publishUpdated(uid);
      gameFlag = true;
    }
    const authFlag = await this._setAuthBan(acc && acc.open_id, uid, false, null);
    await this.audit('unban', uid, { gameFlag, authFlag });
    return { ok: true, uid, gameFlag, authFlag };
  }

  // --- internals ------------------------------------------------------------

  _publishUpdated(uid) {
    if (!this.bus) return;
    this.bus.publishPS('player.updated', 'PlayerUpdated', { account_id: Number(uid) })
      .catch((e) => logger.warn(`[admin] player.updated uid=${uid}: ${e.message}`));
  }

  // Cross-process kick: gateway drops the TCP socket, match server ejects the UDP
  // session (both subscribe to session.revoke). Reports whether the player was
  // online anywhere (presence lookup).
  async _revoke(uid, reason) {
    let wasOnline = false, node = null;
    if (this.bus) {
      node = await this.bus.getNode(uid).catch(() => null);
      wasOnline = !!node;
      await this.bus.publishPS('session.revoke', 'SessionRevoke', { account_id: Number(uid), reason: String(reason) })
        .catch((e) => logger.warn(`[admin] session.revoke uid=${uid}: ${e.message}`));
    }
    return { wasOnline, node };
  }

  // Write the auth-store ban flag (best-effort). Matches by uid first (indexed),
  // then open_id. Returns { ok, rows } or { ok:false, ... }.
  async _setAuthBan(openId, uid, banned, reason) {
    if (!this.pool) return { ok: false, skipped: 'no DATABASE_URL' };
    const rsn = banned ? String(reason) : null;
    try {
      const r = await this.pool.query(
        `UPDATE ${this.schema}.accounts SET banned = $2, ban_reason = $3 WHERE uid = $1`,
        [uid, banned, rsn]
      );
      if (r.rowCount === 0 && openId) {
        const r2 = await this.pool.query(
          `UPDATE ${this.schema}.accounts SET banned = $2, ban_reason = $3 WHERE open_id = $1`,
          [openId, banned, rsn]
        );
        return { ok: true, rows: r2.rowCount };
      }
      return { ok: true, rows: r.rowCount };
    } catch (e) {
      logger.warn(`[admin] auth ban write uid=${uid}: ${e.message}`);
      return { ok: false, error: e.message };
    }
  }

  async _ensureAudit() {
    if (!this.pool || this._auditReady) return;
    await this.pool.query(`
      CREATE TABLE IF NOT EXISTS public.admin_audit (
        id        BIGSERIAL PRIMARY KEY,
        ts        TIMESTAMPTZ NOT NULL DEFAULT now(),
        actor     TEXT,
        action    TEXT,
        target_id BIGINT,
        args      JSONB,
        result    TEXT
      )`);
    this._auditReady = true;
  }

  async audit(action, targetId, args, result = 'ok') {
    if (!this.pool) return;
    try {
      await this._ensureAudit();
      await this.pool.query(
        `INSERT INTO public.admin_audit (actor, action, target_id, args, result)
         VALUES ($1, $2, $3, $4::jsonb, $5)`,
        [this.actor, action, targetId != null ? Number(targetId) : null, JSON.stringify(args || {}), result]
      );
    } catch (e) {
      logger.warn(`[admin] audit ${action}: ${e.message}`);
    }
  }

  async close() {
    try { if (this.bus) await this.bus.close(); } catch (e) { /* ignore */ }
    try { if (this.pool) await this.pool.end(); } catch (e) { /* ignore */ }
    try { if (this.repo && this.repo.close) await this.repo.close(); } catch (e) { /* ignore */ }
    try { await authStore.close(); } catch (e) { /* ignore */ }
  }
}

module.exports = { AdminCore };
