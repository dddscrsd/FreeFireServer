'use strict';

// ROOM / ROOMLIST (RoomListReq -> RoomListRes). Returns the WAITING rooms as RoomBasicInfo.
const { EProtocol, ECustomRoom } = require('../protocol');
const rooms = require('../rooms');

async function handler(reqObj, ctx) {
  const room_list = await rooms.list(reqObj);
  ctx.logger.info(`[tcp] RoomList uid=${ctx.account.uid} -> ${room_list.length} room(s)`);
  return { room_list };
}

module.exports = {
  protocol: EProtocol.ROOM,        // 14
  subcmd: ECustomRoom.ROOMLIST,    // 1
  reqType: 'tcp.RoomListReq',
  resType: 'tcp.RoomListRes',
  handler
};
