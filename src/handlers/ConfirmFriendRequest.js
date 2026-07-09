/**
 * ConfirmFriendRequest (CSFriendReq -> AccountInfoWithPresence)
 * reference: confirm_friend_request @ htpp.py:6686 — addee (current account)
 * accepts adder's request: drop from requests, add mutual friendship, return the
 * now-confirmed friend's info. If the adder is online, push a CONFIRM_NTF so their
 * friend list updates live (the friendship is already mutual in the DB either way).
 *
 * Ported from ported_9.js (handleConfirmFriendRequest).
 */

'use strict';

const { requireAccount, nowSecs, buildAccountInfoBasic } = require('./_shared');
const { getRepo } = require('../db/repo');
const { getBus } = require('../bus/instance');
const { lookup } = require('../protocol/protos');
const { EProtocol, EFriend } = require('../tcp/protocol');

async function handleConfirmFriendRequest(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const adderId = Number(reqObj.adder || 0);
  if (!adderId) return {};

  account.friends = Array.isArray(account.friends) ? account.friends : [];
  account.requests = Array.isArray(account.requests) ? account.requests : [];
  account.requests = account.requests.filter((r) => Number(r) !== adderId);
  if (!account.friends.some((f) => Number(f) === adderId)) account.friends.push(adderId);
  ctx.savePlayer();

  // Mirror the friendship onto the adder if they exist in our store.
  const adder = await getRepo().getById(adderId);
  if (adder) {
    adder.friends = Array.isArray(adder.friends) ? adder.friends : [];
    if (!adder.friends.some((f) => Number(f) === Number(account.uid))) {
      adder.friends.push(Number(account.uid));
      await getRepo().save(adder);
    }
  }

  // Real-time: tell the adder (the original requester), if online, that we accepted —
  // so we appear in their friend list live instead of only after a re-open. Carries
  // OUR AccountInfoBasic (the shape the working REQUEST_NTF uses). Best-effort; the
  // friendship is already persisted mutually, so a missed push just means "on reopen".
  try {
    const bus = getBus();
    if (bus && (await bus.getNode(adderId))) {
      const T = lookup('AccountInfoBasic');
      if (T) {
        const content = Buffer.from(T.encode(T.fromObject(buildAccountInfoBasic(account))).finish());
        await bus.publishPS('gw.push', 'GatewayPush', {
          target_account_id: adderId,
          protocol: EProtocol.FRIEND,
          cmd: EFriend.CONFIRM_NTF,
          content
        });
        ctx.logger.info(`[ported_9] pushed CONFIRM_NTF -> uid=${adderId} (accepter=${account.uid})`);
      }
    }
  } catch (e) {
    ctx.logger.warn(`[ported_9] confirm push failed: ${e.message}`);
  }

  const friend = adder;
  if (!friend) return { account_id: adderId, update_time: nowSecs() };

  const si = friend.selected_items || {};
  return {
    account_id: friend.uid,
    account_type: friend.account_type || 0,
    nickname: friend.nickname || '',
    external_id: friend.open_id || '',
    region: friend.region || '',
    portrait: String(si.head_pic || 0),
    level: friend.level || 1,
    exp: friend.exp || 0,
    update_time: nowSecs()
  };
}

module.exports = {
  endpoint: 'ConfirmFriendRequest',
  reqType: null,
  resType: null,
  handler: handleConfirmFriendRequest
};
