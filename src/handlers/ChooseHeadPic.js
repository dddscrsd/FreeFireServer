/**
 * ChooseHeadPic  (CSChooseHeadPicReq -> CSGetSelectedItemsRes)  [ref @2916]
 *
 * Persist selected_items.head_pic, then return the full selected-items bundle.
 */

'use strict';

const { requireAccount, buildSelectedItems } = require('./_shared');

function handleChooseHeadPic(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  if (reqObj && reqObj.head_pic != null) {
    account.selected_items = account.selected_items || {};
    account.selected_items.head_pic = reqObj.head_pic;
    ctx.savePlayer();
  }
  return { items: buildSelectedItems(account) };
}

module.exports = {
  endpoint: 'ChooseHeadPic',
  reqType: null,
  resType: null,
  handler: handleChooseHeadPic
};
