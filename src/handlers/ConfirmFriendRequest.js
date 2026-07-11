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

const { requireAccount, nowSecs, accountToPresence } = require('./_shared');
const { getRepo } = require('../db/repo');
const { getBus } = require('../bus/instance');
const { lookup } = require('../protocol/protos');
const { EProtocol, EFriend } = require('../tcp/protocol');

async function handleConfirmFriendRequest(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const adderId = Number(reqObj.adder || 0);
  if (!adderId) return {};

  // Establish the mutual friendship and clear the pending request, atomically (relational).
  await getRepo().addFriendship(account.uid, adderId);

  // Real-time: tell the adder (the original requester), if online, that we accepted — so
  // we appear in their friend list live instead of only after a re-open. The client's
  // CONFIRM_NTF handler (OnMsgFriend_ConfirmAdd) decodes the content as
  // AccountInfoWithPresence and builds the new-friend row STRAIGHT from it (no re-fetch),
  // so it MUST be that type — AccountInfoBasic silently mis-parses. Best-effort; the
  // friendship is already persisted mutually, so a missed push just means "on reopen".
  try {
    const bus = getBus();
    if (bus && (await bus.getNode(adderId))) {
      const T = lookup('AccountInfoWithPresence');
      if (T) {
        const content = Buffer.from(T.encode(T.fromObject(accountToPresence(account, nowSecs()))).finish());
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

  const friend = await getRepo().getById(adderId);
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
