/**
 * DeclineFriendRequest  (CSFriendReq -> Empty)  [ref @6858]
 *
 * Ported from ported_1.js (handleDeclineFriendRequest).
 * The authed account is the addee declining; drop adder's pending request.
 */

'use strict';

const { requireAccount } = require('./_shared');
const { getRepo } = require('../db/repo');

async function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const adder = reqObj && reqObj.adder;
  if (adder != null) {
    await getRepo().removeRequest(account.uid, Number(adder));
    ctx.logger.info(`[ported_1] DeclineFriendRequest uid=${account.uid} adder=${adder}`);
  }
  return {};
}

module.exports = {
  endpoint: 'DeclineFriendRequest',
  reqType: 'CSFriendReq',
  resType: 'Empty',
  handler
};
