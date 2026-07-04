/**
 * GetRoomList (RoomListReq -> RoomListRes)
 * reference: handle_GetRoomList @ htpp.py:6059 — empty stub.
 *
 * Ported from ported_9.js (handleGetRoomList), preserving logic exactly.
 */

'use strict';

function handleGetRoomList() {
  return { room_list: [] };
}

module.exports = {
  endpoint: 'GetRoomList',
  reqType: 'RoomListReq',
  resType: 'RoomListRes',
  handler: handleGetRoomList
};
