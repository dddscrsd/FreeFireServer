/**
 * Logout  (empty -> Empty)
 *
 * Ported from ported_7.js (handleLogout).
 * reference: logout @ htpp.py:4490 returns empty. We additionally invalidate
 * the stored login token so the session can no longer authenticate (basic).
 */

'use strict';

const { requireAccount } = require('./_shared');

function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (account) {
    account.token = null;
    account.token_created_at = 0;
    ctx.savePlayer();
    ctx.logger.info(`[ported_7] Logout uid=${account.uid}`);
  }
  return {};
}

module.exports = {
  endpoint: 'Logout',
  reqType: null,
  resType: 'Empty',
  handler
};
