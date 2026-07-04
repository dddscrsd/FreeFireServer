/**
 * GetBackpack  (CSGetBackpackReq -> CSGetBackpackRes)
 * reference: get_backpack @ htpp.py:3135 — return the player's wallet,
 * selected_items (synced from the selected profile) and backpack items.
 *
 * Ported from ported_0.js (handleGetBackpack), preserving logic exactly.
 */

'use strict';

const path = require('path');
const { requireAccount, selectedProfile, buildSelectedItems } = require('./_shared');
const GLOBAL_ITEMS = require(path.join(__dirname, '..', '..', 'config', 'items.json')).items;

function handleGetBackpack(reqObj, ctx) {
    const account = requireAccount(ctx);
    if (!account) return {};

    const prof = selectedProfile(account);
    if (prof) {
        account.selected_items = account.selected_items || {};
        account.selected_items.avatar_id = prof.avatar_id;
        account.selected_items.skin_color = prof.skin_color;
        account.selected_items.clothes = Array.isArray(prof.clothes) ? prof.clothes : [];
        ctx.savePlayer();
    }

    const wallet = account.wallet || {
        coins: account.coins || 0,
        gems: account.gems || 0
    };

    let items = GLOBAL_ITEMS;

    const epBadgeId = account.ep_badge_id || 1001000021;
    const epBadgeCount = account.ep_badge_count || 999;

    ctx.logger.info(`[backpack] GetBackpack uid=${account.uid} epBadgeId=${epBadgeId} epBadgeCount=${epBadgeCount} size=${items.length}`);

    items = items.filter(item => item.id !== epBadgeId);

    ctx.logger.info(`[backpack] GetBackpack uid=${account.uid} filtered size=${items.length}`);
    if (epBadgeCount > 0) {
        items.push({
            id: epBadgeId,
            cnt: epBadgeCount,
            expire_time: 0,
            left_use_times: -1,
            history_owned_cnt: epBadgeCount,
            item_status: 1
        });
    }

    ctx.logger.info(`[backpack] GetBackpack uid=${account.uid} final size=${items.length}`);
    return {
        wallet: {
            coins: wallet.coins || 0,
            gems: wallet.gems || 0
        },
        selected_items: buildSelectedItems(account),
        items
    };
}

module.exports = {
  endpoint: 'GetBackpack',
  reqType: 'CSGetBackpackReq',
  resType: 'CSGetBackpackRes',
  handler: handleGetBackpack
};
