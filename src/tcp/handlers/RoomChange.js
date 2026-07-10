'use strict';

// ROOM / CHANGE (RoomChangeReq -> RoomInfo). Owner-only settings edit. The settings
// (room_setting / room_setting2 / cs_advanced_setting) are stored OPAQUE and rebroadcast —
// they are decoded match-side at start (later phase). Others get CHANGE_NTF(14) = RoomInfo;
// the owner gets the updated RoomInfo under CHANGE(13).
const { EProtocol, ECustomRoom } = require('../protocol');
const rooms = require('../rooms');

async function handler(reqObj, ctx) {
  return rooms.runOp(ctx, async () => {
    const { room } = await rooms.change(ctx.account, reqObj);
    rooms.broadcast(room, ctx.account.uid, ECustomRoom.CHANGE_NTF, 'tcp.RoomInfo', rooms.toRoomInfo(room));
    return rooms.toRoomInfo(room);
  });
}

module.exports = {
  protocol: EProtocol.ROOM,       // 14
  subcmd: ECustomRoom.CHANGE,     // 13
  reqType: 'tcp.RoomChangeReq',
  resType: 'tcp.RoomInfo',
  handler
};
