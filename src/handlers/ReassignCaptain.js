/**
 * ReassignCaptain  (CSReassignCaptainReq -> Empty)
 *   reference: empty stub @ htpp.py:6004. port_resolve -> reqType
 *   CSReassignCaptainReq (reassignee_id); no response message exists -> Empty.
 *
 * Ported from ported_8.js (handleReassignCaptain), preserving logic exactly.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleReassignCaptain(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  ctx.logger.info(
    `[ported] ReassignCaptain uid=${account.uid} reassignee=${reqObj.reassignee_id}`
  );
  return {};
}

module.exports = {
  endpoint: 'ReassignCaptain',
  reqType: 'CSReassignCaptainReq',
  resType: 'Empty',
  handler: handleReassignCaptain
};
