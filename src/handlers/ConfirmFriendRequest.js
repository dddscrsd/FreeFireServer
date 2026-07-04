/**
 * ConfirmFriendRequest (CSFriendReq -> AccountInfoWithPresence)
 * reference: confirm_friend_request @ htpp.py:6686 — addee (current account)
 * accepts adder's request: drop from requests, add mutual friendship, return
 * the now-confirmed friend's info.
 *
 * Ported verbatim from ported_9.js (handleConfirmFriendRequest). Registered
 * without explicit req/res types in the source, so both are null.
 */

'use strict';

const { requireAccount, nowSecs } = require('./_shared');

function handleConfirmFriendRequest(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const adderId = Number(reqObj.adder || 0);
  if (!adderId) return {};

  account.friends = Array.isArray(account.friends) ? account.friends : [];
  account.requests = Array.isArray(account.requests) ? account.requests : [];
  account.requests = account.requests.filter((r) => Number(r) !== adderId);
  if (!account.friends.some((f) => Number(f) === adderId)) account.friends.push(adderId);
  ctx.savePlayer();

  // Mirror the friendship onto the adder if they exist in our store.
  const player = require('../db/player');
  const adder = player.getById(adderId);
  if (adder) {
    adder.friends = Array.isArray(adder.friends) ? adder.friends : [];
    if (!adder.friends.some((f) => Number(f) === Number(account.uid))) {
      adder.friends.push(Number(account.uid));
      player.save(adder);
    }
  }

  const friend = adder;
  if (!friend) return { account_id: adderId, update_time: nowSecs() };

  const si = friend.selected_items || {};
  return {
    account_id: friend.uid,
    account_type: friend.account_type || 0,
    nickname: friend.nickname || '',
    external_id: friend.open_id || '',
    region: friend.region || '',
    portrait: String(si.head_pic || 0),
    level: friend.level || 1,
    exp: friend.exp || 0,
    update_time: nowSecs()
  };
}

module.exports = {
  endpoint: 'ConfirmFriendRequest',
  reqType: null,
  resType: null,
  handler: handleConfirmFriendRequest
};
