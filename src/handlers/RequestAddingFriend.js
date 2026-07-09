/**
 * RequestAddingFriend  (CSFriendReq -> Empty)
 *
 * Ported verbatim from ported_2.js (handleRequestAddingFriend).
 * reference: @6593 — add the adder to the addee's `requests` list (dedup),
 * persisting on the target account. Then, if the addee is online, push a
 * real-time friend-request notification to their gateway (cross-layer).
 */

'use strict';

const { getRepo } = require('../db/repo');
const { requireAccount, buildAccountInfoBasic } = require('./_shared');
const { getBus } = require('../bus/instance');
const { lookup } = require('../protocol/protos');
const { EProtocol, EFriend } = require('../tcp/protocol');

async function handleRequestAddingFriend(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const adder = reqObj.adder || account.uid;
  const addee = reqObj.addee;
  if (!adder || !addee) return {};

  const target = Number(addee) === Number(account.uid) ? account : await getRepo().getById(addee);
  if (!target) return {};
  target.requests = target.requests || [];
  if (!target.requests.includes(adder)) target.requests.push(adder);
  await getRepo().save(target);
  ctx.logger.info(`[ported_2] RequestAddingFriend ${adder} -> ${addee}`);

  // Real-time push: if the addee is ONLINE (presence), tell whichever gateway holds
  // their connection to raise the friend-request notification now (protocol FRIEND /
  // REQUEST_NTF) instead of the client only discovering it on its next
  // GetFriendRequestList poll. The push carries the adder's AccountInfoBasic (the same
  // shape the request list uses); the client re-reads the list over HTTP regardless,
  // so this is purely a latency win. Best-effort — never affects the response.
  try {
    const bus = getBus();
    if (bus && (await bus.getNode(Number(addee)))) {
      const adderAcc = Number(adder) === Number(account.uid) ? account : await getRepo().getById(adder);
      const T = lookup('AccountInfoBasic');
      if (adderAcc && T) {
        const content = Buffer.from(T.encode(T.fromObject(buildAccountInfoBasic(adderAcc))).finish());
        await bus.publishPS('gw.push', 'GatewayPush', {
          target_account_id: Number(addee),
          protocol: EProtocol.FRIEND,
          cmd: EFriend.REQUEST_NTF,
          content
        });
        ctx.logger.info(`[ported_2] pushed REQUEST_NTF -> uid=${addee} (adder=${adder})`);
      }
    }
  } catch (err) {
    ctx.logger.warn(`[ported_2] friend-request push failed: ${err.message}`);
  }
  return {};
}

module.exports = {
  endpoint: 'RequestAddingFriend',
  reqType: 'CSFriendReq',
  resType: 'Empty',
  handler: handleRequestAddingFriend
};
