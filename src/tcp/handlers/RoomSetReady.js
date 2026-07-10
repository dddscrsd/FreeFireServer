'use strict';

// ROOM / SETREADY (RoomSetReadyReq -> empty ack). Toggle the caller's ready flag; the other
// members get SETREADY_NTF(25) with the full player list so their roster ticks update.
const { EProtocol, ECustomRoom } = require('../protocol');
const rooms = require('../rooms');

async function handler(reqObj, ctx) {
  return rooms.runOp(ctx, async () => {
    const { room } = await rooms.setReady(ctx.account, !!reqObj.ready);
    rooms.broadcast(room, ctx.account.uid, ECustomRoom.SETREADY_NTF, 'tcp.RoomSetReadyNtf', rooms.setReadyNtf(room));
    return {}; // empty ack under SETREADY(24)
  });
}

module.exports = {
  protocol: EProtocol.ROOM,        // 14
  subcmd: ECustomRoom.SETREADY,    // 24
  reqType: 'tcp.RoomSetReadyReq',
  handler
};
