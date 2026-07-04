/**
 * GetBroadcastList  (-> CSGetBroadcastListRes)  [ref @4453]
 * Lobby scrolling broadcast messages. Ported from ported_1.js.
 */

'use strict';

const { nowMs } = require('./_shared');

function handleGetBroadcastList(reqObj, ctx) {
  ctx.logger.info('[ported_1] GetBroadcastList');
  return {
    broadcast_messages: [
      {
        nickname: 'System',
        navigation_type: 0,
        source: 'Welcome to the server!',
        item_id: 0,
        time_stamp: nowMs(),
        source_id: 1
      },
      {
        nickname: 'Admin',
        navigation_type: 1,
        source: 'Check out what is new in the shop!',
        item_id: 1001,
        time_stamp: nowMs() - 3600 * 1000,
        source_id: 2
      }
    ],
    silence_show_switch: false
  };
}

module.exports = {
  endpoint: 'GetBroadcastList',
  reqType: null,
  resType: 'CSGetBroadcastListRes',
  handler: handleGetBroadcastList
};
