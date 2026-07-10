'use strict';

// ROOM / JOIN (RoomJoinReq -> RoomInfo). Join by room_id or join-code. The joiner gets the
// full RoomInfo under JOIN(3); the existing members get JOIN_NTF(5) (which carries room_info
// so their roster refreshes) via the presence-routed gw.push.
const { EProtocol, ECustomRoom } = require('../protocol');
const rooms = require('../rooms');

async function handler(reqObj, ctx) {
  return rooms.runOp(ctx, async () => {
    const { room, joiner } = await rooms.join(ctx.account, reqObj);
    rooms.broadcast(room, ctx.account.uid, ECustomRoom.JOIN_NTF, 'tcp.RoomJoinNtf', rooms.joinNtf(room, joiner));
    return rooms.toRoomInfo(room);
  });
}

module.exports = {
  protocol: EProtocol.ROOM,        // 14
  subcmd: ECustomRoom.JOIN,        // 3
  reqType: 'tcp.RoomJoinReq',
  resType: 'tcp.RoomInfo',
  handler
};
