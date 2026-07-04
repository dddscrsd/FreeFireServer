/**
 * DeclineFriendRequest  (CSFriendReq -> Empty)  [ref @6858]
 *
 * Ported verbatim from ported_1.js (handleDeclineFriendRequest).
 * The authed account is the addee declining; remove adder from its requests.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const adder = reqObj && reqObj.adder;
  if (adder != null && Array.isArray(account.requests)) {
    const before = account.requests.length;
    account.requests = account.requests.filter(
      (r) => String(r) !== String(adder) && String(r && r.account_id) !== String(adder)
    );
    if (account.requests.length !== before) ctx.savePlayer();
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
