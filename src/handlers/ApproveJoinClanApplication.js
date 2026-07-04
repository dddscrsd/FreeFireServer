/**
 * ApproveJoinClanApplication  (CSProcessJoinClanReq -> Empty)
 *
 * Ported from ported_2.js (emptyStub).
 * port_resolve: clan-application approve. Resolved reqType from the
 * applicant_id/clan_id shape -> CSProcessJoinClanReq; resType Empty. The
 * reference handler returns an empty body; we return a default-constructed res.
 */

'use strict';

// Empty-stub endpoint: reference handler returns an empty body; we return a
// default-constructed resType. Auth is enforced by the router.
function emptyStub() {
  return {};
}

module.exports = {
  endpoint: 'ApproveJoinClanApplication',
  reqType: 'CSProcessJoinClanReq',
  resType: 'Empty',
  handler: emptyStub
};
