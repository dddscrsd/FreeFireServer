/**
 * MajorRegister  (PlatformRegisterReq -> PlatformRegisterRes / MajorRegisterRes)
 * reference: platform_register @ htpp.py:1903. Basic validation only.
 *
 * Ported verbatim from login.js (handleMajorRegister).
 */

'use strict';

const player = require('../db/player');
const { getRepo } = require('../db/repo');
const gate = require('./_authGate');

async function handleMajorRegister(reqObj, ctx) {
  const openId = player.deriveOpenId(reqObj);

  // Registration hardening. NOTE: this request is a PlatformRegisterReq, which
  // carries NEITHER client_version NOR signature_md5 (those live on LoginReq /
  // MajorLogin) — so the version/signature gates do NOT apply here; running them
  // would 403 every register the moment an allow-list is set. Only the
  // registration + ban gates apply. The login token is named `access_token` on
  // this request (it's `login_token` on LoginReq), so read it from there.
  const loginToken = reqObj.access_token || reqObj.login_token || '';
  ctx.logger.info(
    `[login] MajorRegister open_id=${openId} access_token_present=${!!loginToken}`
  );
  const reg = await gate.checkRegistration(openId, loginToken);
  if (reg) return gate.reject(ctx, reg);
  const banned = await gate.checkBan(openId, null);
  if (banned) return gate.reject(ctx, banned);

  const account = await getRepo().createFromLogin(reqObj);
  ctx.logger.info(`[login] MajorRegister uid=${account.uid} open_id=${account.open_id}`);
  return {
    account_id: account.uid,
    first_game_open: false,
    br_tutorial_open: false,
    cs_tutorial_open: false
  };
}

module.exports = {
  endpoint: 'MajorRegister',
  reqType: 'PlatformRegisterReq',
  resType: 'PlatformRegisterRes',
  handler: handleMajorRegister
};
