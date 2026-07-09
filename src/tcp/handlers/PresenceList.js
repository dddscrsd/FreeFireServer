'use strict';

/**
 * PRESENCE / PRESENCELIST  (PresenceListReq -> PresenceListRes)
 *
 * The client periodically asks for the online status of a set of account_ids (its
 * friends / clan) to draw the online dots — this is the ACTUAL source of friend
 * status, not GetFriend. We answer from the cross-layer presence keys the gateways
 * write to Redis (presence:<uid>), so a friend connected to ANY gateway node reads
 * as online. PresenceInfo.p: 0=NONE(offline), 1=ONLINE (INGROUP/INGAME/INROOM are
 * future refinements). reference: handle_presence @ tcpp.py:4090.
 */

const { EProtocol, EPresence } = require('../protocol');
const { getBus } = require('../../bus/instance');
const { DEFAULT_REGION } = require('../../handlers/_shared');

async function handler(reqObj, ctx) {
  const ids = (Array.isArray(reqObj.account_ids) ? reqObj.account_ids : []).map(Number).filter(Boolean);
  const listType = reqObj.account_list_type || 0;

  let online = {};
  try {
    const bus = getBus();
    if (bus && ids.length) online = await bus.getNodes(ids);
  } catch (e) {
    ctx.logger.warn(`[tcp] PresenceList lookup failed: ${e.message}`);
  }

  const presences = ids.map((id) => ({ i: id, r: ctx.region || DEFAULT_REGION, p: online[id] ? 1 : 0 }));
  ctx.logger.info(`[tcp] PresenceList uid=${ctx.account.uid} queried=${ids.length} online=${presences.filter((x) => x.p).length} region=${ctx.region || DEFAULT_REGION}`);
  return { presences, account_list_type: listType };
}

module.exports = {
  protocol: EProtocol.PRESENCE,     // 15
  subcmd: EPresence.PRESENCELIST,   // 1
  reqType: 'PresenceListReq',
  resType: 'PresenceListRes',
  handler
};
