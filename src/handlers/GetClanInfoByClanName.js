/**
 * GetClanInfoByClanName  (CSClanNameReq -> ClanInfo)  [resolved]
 *
 * Ported verbatim from ported_7.js (emptyStub('GetClanInfoByClanName')).
 * reference: handle_GetClanInfoByClanName @ htpp.py:6019 — empty stub. Faithful
 * port = default-constructed resType.
 */

'use strict';

function handler(reqObj, ctx) {
  ctx.logger.info('[ported_7] GetClanInfoByClanName -> default res');
  return {};
}

module.exports = {
  endpoint: 'GetClanInfoByClanName',
  reqType: 'CSClanNameReq',
  resType: 'ClanInfo',
  handler
};
