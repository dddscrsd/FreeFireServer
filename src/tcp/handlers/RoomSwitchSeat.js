'use strict';

// ROOM / SWITCHSEAT (RoomSwitchSeatReq). The player clicks an empty seat to move there.
// WIRE QUIRK (RE-confirmed): the field names are SWAPPED — to_room_pos = TEAM ordinal (0-based),
// to_group_pos = SEAT within team (0-based). to_role MEMBER=1 for a seat move.
//
// The client does NOT act on a subcmd-20 reply (it's in OnMsgCustomRoom's no-op default set);
// the seat move is applied by broadcasting SWITCHSEAT_NTF(21) = tcp.RoomJoinNtf{room_info}, which
// the client feeds to UpdateCurrentRoomInfo -> full grid re-render. So we ack subcmd 20 (ignored)
// and push the updated RoomInfo as RoomJoinNtf(21) to EVERY member incl. the mover.
const { EProtocol, ECustomRoom } = require('../protocol');
const rooms = require('../rooms');

async function handler(reqObj, ctx) {
  return rooms.runOp(ctx, async () => {
    const { room } = await rooms.switchSeat(ctx.account, reqObj);
    const ntf = { join_player_list: [], room_info: rooms.toRoomInfo(room) };
    // exceptId=null -> reaches all real members (none has account_id 0), including the mover,
    // which needs the NTF since subcmd 20 alone doesn't move its own seat.
    rooms.broadcast(room, null, ECustomRoom.SWITCHSEAT_NTF, 'tcp.RoomJoinNtf', ntf);
    return {}; // ack under SWITCHSEAT(20) (client ignores it, harmless)
  });
}

module.exports = {
  protocol: EProtocol.ROOM,            // 14
  subcmd: ECustomRoom.SWITCHSEAT,      // 20
  reqType: 'tcp.RoomSwitchSeatReq',
  handler
};
