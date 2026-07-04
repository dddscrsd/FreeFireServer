/**
 * DeclineJoinClanApplication  (CSProcessJoinClanReq -> Empty)
 *
 * Ported from ported_2.js (emptyStub). reference @5984: empty stub returning an
 * empty body. port_resolve: clan-application decline. Resolved reqType from the
 * applicant_id/clan_id shape -> CSProcessJoinClanReq; resType Empty.
 */

'use strict';

function handler() {
  return {};
}

module.exports = {
  endpoint: 'DeclineJoinClanApplication',
  reqType: 'CSProcessJoinClanReq',
  resType: 'Empty',
  handler
};
