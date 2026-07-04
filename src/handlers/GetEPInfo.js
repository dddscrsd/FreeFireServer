/**
 * GetEPInfo  (Empty -> CSGetEPInfoRes)
 *
 * Ported verbatim from ported_5.js (handleGetEPInfo).
 * reference: build_get_ep_info_protobuf @ htpp.py:3260.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleGetEPInfo(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const now = Math.floor(Date.now() / 1000);
  const epEventId = account.ep_event_id || 21;
  const count = account.ep_badge_count || 999;

  const rewards = [];
  for (let level = 1; level <= 50; level++) {
    rewards.push({ unlock_id: level, is_ep: 0 });
    rewards.push({ unlock_id: level + 100, is_ep: 1 });
  }

  const challenges = [
    { challenge_id: 1, update_time: now, count_type: 3 },
    { challenge_id: 2, update_time: now, count_type: 16 }
  ];

  return {
    owned_pass: account.elite_pass != null ? !!account.elite_pass : true,
    owned_fp_challenge: account.owned_fp_challenge != null ? !!account.owned_fp_challenge : true,
    ep_event_id: epEventId,
    ep_badge: 1001000000 + epEventId,
    gold_limit_improved: count,
    fp_challenge_item: count,
    purchase_badge_count_today: count,
    week: count,
    daily_reset_time: (Math.floor((now + 86400) / 86400)) * 86400,
    epid_preorder: account.epid_preorder || 0,
    ep_preorder_start_time: account.ep_preorder_start_time || 0,
    ep_preorder_end_time: account.ep_preorder_end_time || 0,
    rewards,
    challenges
  };
}

module.exports = {
  endpoint: 'GetEPInfo',
  reqType: 'Empty',
  resType: 'CSGetEPInfoRes',
  handler: handleGetEPInfo
};
