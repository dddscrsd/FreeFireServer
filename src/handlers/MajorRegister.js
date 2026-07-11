/**
 * MajorRegister  (PlatformRegisterReq -> PlatformRegisterRes / MajorRegisterRes)
 * reference: platform_register @ htpp.py:1903. Basic validation only.
 *
 * Ported verbatim from login.js (handleMajorRegister).
 */

'use strict';

const { getRepo } = require('../db/repo');

async function handleMajorRegister(reqObj, ctx) {
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
