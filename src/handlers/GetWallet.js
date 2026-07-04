/**
 * GetWallet  (idx 108, py 4340)  -> CSGetWalletRes
 *
 * reference: empty stub, but our wallet response carries coins/gems. Build it
 * from the player's wallet/balances.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleGetWallet(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const wallet = account.wallet || {};
  return {
    account_id: account.uid,
    wallet: {
      coins: wallet.coins != null ? wallet.coins : account.coins || 0,
      gems: wallet.gems != null ? wallet.gems : account.gems || 0,
      gop_gems: 0,
      total_topup: 0,
      last_topup_time: 0
    }
  };
}

module.exports = {
  endpoint: 'GetWallet',
  reqType: null,
  resType: null,
  handler: handleGetWallet
};
