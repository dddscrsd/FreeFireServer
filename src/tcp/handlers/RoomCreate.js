'use strict';

// ROOM / CREATE (RoomCreateReq -> RoomInfo). The host creates a private room; the reply is
// the full RoomInfo under CREATE(2) (the client's RequestCreateRoom hides its waiting dialog
// on this subcmd). No broadcast — the owner is the only member.
const { EProtocol, ECustomRoom } = require('../protocol');
const rooms = require('../rooms');

async function handler(reqObj, ctx) {
  return rooms.runOp(ctx, async () => {
    const room = await rooms.create(ctx.account, reqObj);
    return rooms.toRoomInfo(room);
  });
}

module.exports = {
  protocol: EProtocol.ROOM,        // 14
  subcmd: ECustomRoom.CREATE,      // 2
  reqType: 'tcp.RoomCreateReq',
  resType: 'tcp.RoomInfo',
  handler
};
