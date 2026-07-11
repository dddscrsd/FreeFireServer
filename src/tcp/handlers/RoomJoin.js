'use strict';

// ROOM / JOIN (RoomJoinReq). Join by room_id or join-code. The JOIN(3) reply is a client NO-OP
// (subcmd 3 is in OnMsgCustomRoom's default set), so we do NOT reply to it. Instead we broadcast
// JOIN_NTF(5) = tcp.RoomJoinNtf{join_player_list, room_info} to ALL members INCLUDING the joiner:
//   - existing members (CheckIsInRoom()==true) -> UpdateCurrentRoomInfo, refresh the roster.
//   - the JOINER (not yet in room) -> its case-5 handler searches join_player_list for its OWN
//     account_id and, on a hit, enters + fires OpenCustomRoom -> PushNavigation<UICustomRoomController>
//     (the room-list -> in-room transition). So join_player_list MUST contain the joiner, else the
//     joiner stays stuck on the room list (the bug this fixes).
const { EProtocol, ECustomRoom } = require('../protocol');
const rooms = require('../rooms');

async function handler(reqObj, ctx) {
  return rooms.runOp(ctx, async () => {
    const { room, joiner } = await rooms.join(ctx.account, reqObj);
    // exceptId=null -> reaches ALL real members (incl. the joiner); joinNtf puts the joiner in
    // join_player_list, which is the joiner's enter-room gate.
    rooms.broadcast(room, null, ECustomRoom.JOIN_NTF, 'tcp.RoomJoinNtf', rooms.joinNtf(room, joiner));
    return null; // JOIN(3) reply is a client no-op; JOIN_NTF(5) above drives both entry + refresh
  });
}

module.exports = {
  protocol: EProtocol.ROOM,        // 14
  subcmd: ECustomRoom.JOIN,        // 3
  reqType: 'tcp.RoomJoinReq',
  handler
};
