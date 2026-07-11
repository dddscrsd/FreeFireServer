/**
 * RemoveFriend  (CSRemoveFriendReq -> CSRemoveFriendRes)
 *
 * Ported from ported_0.js (handleRemoveFriend).
 * reference: handle_RemoveFriend @ htpp.py:5810 — empty stub. We drop the friendship
 * on BOTH sides (relational removeFriendship; the old blob version was one-sided and
 * could desync) and echo the ids back.
 */

'use strict';

const { requireAccount, DEFAULT_REGION } = require('./_shared');
const { getRepo } = require('../db/repo');

async function handleRemoveFriend(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const remover = reqObj.remover || account.uid;
  const removee = reqObj.removee || 0;
  if (removee) await getRepo().removeFriendship(account.uid, Number(removee));
  const region = account.region || DEFAULT_REGION;
  return { remover, removee, lock_region: region, noti_region: region };
}

module.exports = {
  endpoint: 'RemoveFriend',
  reqType: 'CSRemoveFriendReq',
  resType: 'CSRemoveFriendRes',
  handler: handleRemoveFriend
};
