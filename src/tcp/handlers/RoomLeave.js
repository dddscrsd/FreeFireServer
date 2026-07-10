'use strict';

// ROOM / LEAVE (RoomLeaveReq -> empty ack). Remove the caller from their room. If the OWNER
// leaves, the room is DISMISSED and every other member gets DISMISS_NTF(8); otherwise the
// remaining members get LEAVE_NTF(7). The leaver gets an empty ack under LEAVE(6).
const { EProtocol, ECustomRoom, ECustomRoomErr } = require('../protocol');
const rooms = require('../rooms');

const DISMISS_NORMAL = 1; // tcp.ERoom.DismissReason: NONE=0, NORMAL=1, OFFLINE=2, TIMEOUT=3, ADMIN=4

async function handler(reqObj, ctx) {
  const res = await rooms.leave(ctx.account.uid);
  if (!res) { ctx.ret = ECustomRoomErr.NOTINROOM; return {}; }
  if (res.dismissed) {
    // res.members includes the leaver — broadcast excludes them by account id.
    rooms.broadcast({ members: res.members }, ctx.account.uid, ECustomRoom.DISMISS_NTF,
      'tcp.RoomDismissNtf', rooms.dismissNtf(res.room, ctx.account.uid, DISMISS_NORMAL));
  } else {
    rooms.broadcast(res.room, ctx.account.uid, ECustomRoom.LEAVE_NTF,
      'tcp.RoomLeaveNtf', rooms.leaveNtf(res.room, res.leaver));
  }
  return {}; // empty ack under LEAVE(6)
}

module.exports = {
  protocol: EProtocol.ROOM,   // 14
  subcmd: ECustomRoom.LEAVE,  // 6
  reqType: 'tcp.RoomLeaveReq',
  handler
};
