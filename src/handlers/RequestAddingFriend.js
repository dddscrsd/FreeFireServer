/**
 * RequestAddingFriend  (CSFriendReq -> Empty)
 *
 * Ported verbatim from ported_2.js (handleRequestAddingFriend).
 * reference: @6593 — add the adder to the addee's `requests` list (dedup),
 * persisting on the target account.
 */

'use strict';

const player = require('../db/player');
const { requireAccount } = require('./_shared');

function handleRequestAddingFriend(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const adder = reqObj.adder || account.uid;
  const addee = reqObj.addee;
  if (!adder || !addee) return {};

  const target = Number(addee) === Number(account.uid) ? account : player.getById(addee);
  if (!target) return {};
  target.requests = target.requests || [];
  if (!target.requests.includes(adder)) target.requests.push(adder);
  player.save(target);
  ctx.logger.info(`[ported_2] RequestAddingFriend ${adder} -> ${addee}`);
  return {};
}

module.exports = {
  endpoint: 'RequestAddingFriend',
  reqType: 'CSFriendReq',
  resType: 'Empty',
  handler: handleRequestAddingFriend
};
