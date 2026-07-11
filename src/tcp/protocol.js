'use strict';

/**
 * Named views over the TCP protocol enums (from tcp.EProtocol / tcp.EMatchmaking,
 * recovered by the proto generator). The generator prefixes value names with the
 * enum name — and some source members were already prefixed — so we strip up to
 * two leading "<EnumName>_" segments to expose clean logical names while also
 * keeping the raw generated name as an alias.
 *
 *   EProtocol.MATCHMAKING            === 3
 *   EMatchmaking.GAMEOPENINGINFO     === 7
 *   EMatchmakingErr.SUSS             === 0
 */

const { lookupEnum } = require('../protocol/protos');

function enumValues(fqn) {
  const e = lookupEnum(fqn);
  if (!e) throw new Error(`[tcp] enum not found: ${fqn}`);
  const simple = fqn.split('.').pop();       // e.g. "Proto" / "ErrCode"
  const pfx = simple + '_';
  const out = {};
  for (const [name, num] of Object.entries(e.values)) {
    let clean = name;
    if (clean.startsWith(pfx)) clean = clean.slice(pfx.length);
    if (clean.startsWith(pfx)) clean = clean.slice(pfx.length);
    out[clean] = num;
    out[name] = num; // keep the raw generated name too
  }
  return out;
}

module.exports = {
  EProtocol: enumValues('tcp.EProtocol.Proto'),
  EFriend: enumValues('tcp.EFriend.Proto'),
  EPresence: enumValues('tcp.EPresence.Proto'),
  EStats: enumValues('tcp.EStats.Proto'),
  EMatchmaking: enumValues('tcp.EMatchmaking.Proto'),
  EMatchmakingErr: enumValues('tcp.EMatchmaking.ErrCode'),
  // Custom rooms ride EProtocol.ROOM (14). The subcmd + error enums are BOTH nested under
  // tcp.ERoom (NOT tcp.ECustomRoom — verified against the loaded protos). enumValues strips
  // the doubled "Proto_Proto_"/"ErrCode_ErrCode_" generator prefix to clean names (CREATE=2,
  // JOIN=3, ROOMINFO=12, CHANGE=13, MATCHMAKINGSUSS_NTF=16, SETREADY=24, …).
  ECustomRoom: enumValues('tcp.ERoom.Proto'),
  ECustomRoomErr: enumValues('tcp.ERoom.ErrCode'),
  enumValues
};
