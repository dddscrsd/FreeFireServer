'use strict';

// ROOM / SWITCHSEAT (RoomSwitchSeatReq). The player clicks an empty seat to move there.
// WIRE QUIRK (RE-confirmed): the field names are SWAPPED — to_room_pos = TEAM ordinal (0-based),
// to_group_pos = SEAT within team (0-based). to_role MEMBER=1 for a seat move.
//
// The client does NOT act on a subcmd-20 reply (it's in OnMsgCustomRoom's no-op default set);
// the move is applied by pushing the updated room. IMPORTANT: the SWITCHSEAT_NTF(21) case decodes
// its payload as a BARE tcp.RoomInfo (NOT RoomJoinNtf) — verified in-client: a RoomJoinNtf here made
// the client read its nested room_info (field 2) as RoomInfo.name (field 2) and join_player_list
// (field 1, empty) as RoomInfo.id -> id=0 + garbled name + wiped room. So we send the full RoomInfo
// directly, exactly like the CREATE(2)/ROOMINFO(12) path, which the client feeds to
// UpdateCurrentRoomInfo -> full grid re-render. Sent to EVERY member incl. the mover.
const { EProtocol, ECustomRoom } = require('../protocol');
const rooms = require('../rooms');

async function handler(reqObj, ctx) {
  return rooms.runOp(ctx, async () => {
    const { room } = await rooms.switchSeat(ctx.account, reqObj);
    // exceptId=null -> reaches all real members (none has account_id 0), including the mover,
    // which needs this since subcmd 20 alone doesn't move its own seat.
    rooms.broadcast(room, null, ECustomRoom.SWITCHSEAT_NTF, 'tcp.RoomInfo', rooms.toRoomInfo(room));
    return {}; // ack under SWITCHSEAT(20) (client ignores it, harmless)
  });
}

module.exports = {
  protocol: EProtocol.ROOM,            // 14
  subcmd: ECustomRoom.SWITCHSEAT,      // 20
  reqType: 'tcp.RoomSwitchSeatReq',
  handler
};
