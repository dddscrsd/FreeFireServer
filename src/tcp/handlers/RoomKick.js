'use strict';

// ROOM / KICK (RoomKickReq -> empty ack). Owner-only. The kicked player gets KICK_NTF(10)
// (so their client exits the room) and the remaining members get KICK_NTF(10) too (room_info
// refreshes their roster). The owner gets an empty ack under KICK(9).
const { EProtocol, ECustomRoom } = require('../protocol');
const rooms = require('../rooms');

async function handler(reqObj, ctx) {
  return rooms.runOp(ctx, async () => {
    const { room, target } = await rooms.kick(ctx.account.uid, reqObj.room_id, reqObj.kick_account_id);
    const ntf = rooms.kickNtf(room, target);
    rooms.pushToAccount(target.account_id, ECustomRoom.KICK_NTF, 'tcp.RoomKickNtf', ntf); // tell the kicked
    rooms.broadcast(room, ctx.account.uid, ECustomRoom.KICK_NTF, 'tcp.RoomKickNtf', ntf);  // refresh the rest
    return {}; // empty ack under KICK(9)
  });
}

module.exports = {
  protocol: EProtocol.ROOM,   // 14
  subcmd: ECustomRoom.KICK,   // 9
  reqType: 'tcp.RoomKickReq',
  handler
};
