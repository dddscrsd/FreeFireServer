/**
 * GetActivityDesc  (CSGetActivityDescReq -> CSGetActivityDescRes)  [ref @2004]
 * Return the welcome-event activity description (mapped to ClientActivityDesc).
 *
 * Ported from ported_1.js (handleGetActivityDesc), preserving logic exactly.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleGetActivityDesc(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  return {
    activity_descs: [
      {
        activity_id: 1,
        group_id: 1,
        act_title: 'Welcome Event',
        act_text: 'Earn exclusive rewards for joining the server!',
        image_url: '',
        image_url_for_lobby: '',
        activity_type: 1,
        sort_id: 1,
        start_time: 1731000000,
        end_time: 1762536000
      }
    ]
  };
}

module.exports = {
  endpoint: 'GetActivityDesc',
  reqType: 'CSGetActivityDescReq',
  resType: 'CSGetActivityDescRes',
  handler: handleGetActivityDesc
};
