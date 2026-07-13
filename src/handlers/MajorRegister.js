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

  // Registration hardening: a client can only register a game account if it
  // passes the version/signature gates AND (when enforced) its open_id is already
  // provisioned on the auth server. Also refuse a banned open_id re-registering.
  ctx.logger.info(
    `[login] MajorRegister open_id=${openId} client_version="${reqObj.client_version || ''}" signature_md5="${reqObj.signature_md5 || ''}"`
  );
  const gated = await gate.runLoginGate(reqObj, openId);
  if (gated) return gate.reject(ctx, gated);
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
