/**
 * MajorLogin  (LoginReq -> MajorLoginRes)  [public]
 *
 * Ported from login.js (handleMajorLogin). LOOK UP the account by open_id only.
 * If it does NOT exist, return HTTP 404 — the client interprets that as "not
 * registered" and opens the register screen, then calls MajorRegister with the
 * chosen nickname. (Matches the reference: _validate_login_common returns
 * make_404_response when the account is missing, htpp.py:2164-2166.) DO NOT
 * auto-create here.
 *
 * When the account exists, issue + STORE a fresh login token so
 * player.getByToken() resolves it on subsequent authed endpoints, and return
 * the region/token/server_url bundle the client needs to proceed to
 * GetLoginData. (reference: major_login @ htpp.py:2201)
 */

'use strict';

const player = require('../db/player');
const { DEFAULT_REGION, TOKEN_TTL, nowSecs, newToken, serverUrl } = require('./_shared');

function handleMajorLogin(reqObj, ctx) {
  const openId = player.deriveOpenId(reqObj);
  const account = player.getByOpenId(openId);
  if (!account) {
    ctx.logger.info(`[login] MajorLogin open_id=${openId} -> 404 (not registered, client will register)`);
    ctx.res.status(404).type('text/plain').end();
    return undefined;
  }

  const token = newToken();
  account.token = token;
  account.token_created_at = nowSecs();
  if (reqObj.device_id) account.device_id = String(reqObj.device_id);
  if (reqObj.client_version) account.client_version = String(reqObj.client_version);
  player.save(account);

  const region = account.region || '';

  ctx.logger.info(
    `[login] MajorLogin uid=${account.uid} open_id=${account.open_id} region="${region}"`
  );

  return {
    account_id: account.uid,
    lock_region: region,
    noti_region: region,
    ip_region: region || DEFAULT_REGION,
    agora_environment: 'live',
    new_active_region: region,
    recommend_regions: [DEFAULT_REGION],
    token,
    ttl: TOKEN_TTL,
    server_url: serverUrl(ctx),
    emulator_score: 0
  };
}

module.exports = {
  endpoint: 'MajorLogin',
  reqType: 'LoginReq',
  resType: 'MajorLoginRes',
  handler: handleMajorLogin
};
