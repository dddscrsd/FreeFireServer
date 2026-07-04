/**
 * SetShowRank  (idx 138, py 7182)  -> CSSetShowRankRes
 *
 * Ported verbatim from ported_8.js (handleSetShowRank).
 * reference: empty stub. Persist the show_rank flag and echo it back.
 */

'use strict';

const { requireAccount } = require('./_shared');

function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const showRank = reqObj.show_rank != null ? !!reqObj.show_rank : true;
  account.show_rank = showRank;
  ctx.savePlayer();
  return { show_rank: showRank };
}

module.exports = {
  endpoint: 'SetShowRank',
  reqType: null,
  resType: null,
  handler
};
