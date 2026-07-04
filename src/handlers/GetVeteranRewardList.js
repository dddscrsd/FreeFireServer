/**
 * GetVeteranRewardList  ((empty) -> CSGetVeteranRewardListRes)  [resolved]
 *
 * reference: handle_GetVeteranRewardList @ htpp.py:7261 (empty stub). Return a
 * default-constructed res indicating the player is not a veteran.
 */

'use strict';

function handleGetVeteranRewardList(reqObj, ctx) {
  ctx.logger.info('[ported_7] GetVeteranRewardList -> default res');
  return { is_veteran: false };
}

module.exports = {
  endpoint: 'GetVeteranRewardList',
  reqType: null,
  resType: 'CSGetVeteranRewardListRes',
  handler: handleGetVeteranRewardList
};
