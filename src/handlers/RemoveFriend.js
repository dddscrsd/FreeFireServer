/**
 * RemoveFriend  (CSRemoveFriendReq -> CSRemoveFriendRes)
 *
 * Ported from ported_0.js (handleRemoveFriend).
 * reference: handle_RemoveFriend @ htpp.py:5810 — empty stub. We drop the
 * removee from the caller's friends list and echo the ids back.
 */

'use strict';

const { requireAccount, DEFAULT_REGION } = require('./_shared');

function handleRemoveFriend(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const remover = reqObj.remover || account.uid;
  const removee = reqObj.removee || 0;
  if (removee && Array.isArray(account.friends)) {
    account.friends = account.friends.filter((f) => {
      const id = typeof f === 'object' ? (f.uid || f.account_id) : f;
      return id !== removee;
    });
    ctx.savePlayer();
  }
  const region = account.region || DEFAULT_REGION;
  return { remover, removee, lock_region: region, noti_region: region };
}

module.exports = {
  endpoint: 'RemoveFriend',
  reqType: 'CSRemoveFriendReq',
  resType: 'CSRemoveFriendRes',
  handler: handleRemoveFriend
};
