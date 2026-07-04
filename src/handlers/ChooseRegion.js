/**
 * ChooseRegion  (CSChooseRegionReq -> CSChooseRegionRes)
 *
 * Ported from login.js (handleChooseRegion). Basic passthrough: persist the
 * requested region onto the account and echo it back.
 */

'use strict';

const player = require('../db/player');
const { requireAccount, DEFAULT_REGION } = require('./_shared');

// CSChooseRegionReq -> CSChooseRegionRes (basic passthrough).
function handleChooseRegion(reqObj, ctx) {
  const account = requireAccount(ctx);
  const region = reqObj.region || DEFAULT_REGION;
  if (account) {
    account.region = region;
    player.save(account);
  }
  return { region };
}

module.exports = {
  endpoint: 'ChooseRegion',
  reqType: 'CSChooseRegionReq',
  resType: 'CSChooseRegionRes',
  handler: handleChooseRegion
};
