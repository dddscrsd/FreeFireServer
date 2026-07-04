/**
 * ChangeMemberTypeByCaptain  (CSChangeMemberTypeReq -> Empty)  [ref @5999]
 *
 * Ported from ported_8.js (handleChangeMemberTypeByCaptain). reference: empty
 * stub. port_resolve -> reqType CSChangeMemberTypeReq (changee_id, target_type);
 * no response message exists -> resType Empty.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleChangeMemberTypeByCaptain(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  ctx.logger.info(
    `[ported] ChangeMemberTypeByCaptain uid=${account.uid} changee=${reqObj.changee_id} type=${reqObj.target_type}`
  );
  return {};
}

module.exports = {
  endpoint: 'ChangeMemberTypeByCaptain',
  reqType: 'CSChangeMemberTypeReq',
  resType: 'Empty',
  handler: handleChangeMemberTypeByCaptain
};
