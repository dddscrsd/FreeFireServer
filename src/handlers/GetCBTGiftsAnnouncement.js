/**
 * GetCBTGiftsAnnouncement  (Empty -> Empty)
 * reference: handle_GetCBTGiftsAnnouncement @ htpp.py:5939 (empty stub). No CBT
 * gifts/announcement message exists in the 1.70 protos, so the response type is
 * unresolved and falls back to proto.Empty (the reference returned empty bytes).
 *
 * Ported verbatim from ported_3.js (handleGetCBTGiftsAnnouncement).
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleGetCBTGiftsAnnouncement(reqObj, ctx) {
  if (!requireAccount(ctx)) return {};
  return {};
}

module.exports = {
  endpoint: 'GetCBTGiftsAnnouncement',
  reqType: 'Empty',
  resType: 'Empty',
  handler: handleGetCBTGiftsAnnouncement
};
