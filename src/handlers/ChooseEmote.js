/**
 * ChooseEmote  (CSChooseEmoteReq -> CSGetSelectedItemsRes)
 *
 * reference: handle_ChooseEmote @ htpp.py:7353 — upsert {slot_id,emote_id} into
 * selected_items.emotes.emotes, persist, return the updated SelectedItems.
 */

'use strict';

const { requireAccount, buildSelectedItems } = require('./_shared');

function handleChooseEmote(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const slotId = reqObj.slot_id || 0;
  const emoteId = reqObj.emote_id || 0;

  account.selected_items = account.selected_items || {};
  account.selected_items.emotes = account.selected_items.emotes || { emotes: [] };
  const list = account.selected_items.emotes.emotes;

  const existing = list.find((e) => e.slot_id === slotId);
  if (existing) existing.emote_id = emoteId;
  else list.push({ slot_id: slotId, emote_id: emoteId });

  ctx.savePlayer();
  ctx.logger.info(`[ported_0] ChooseEmote uid=${account.uid} slot=${slotId} emote=${emoteId}`);

  return { items: buildSelectedItems(account) };
}

module.exports = {
  endpoint: 'ChooseEmote',
  reqType: 'CSChooseEmoteReq',
  resType: 'CSGetSelectedItemsRes',
  handler: handleChooseEmote
};
