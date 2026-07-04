/**
 * LoginGetBroadcast  (CSScrollMarqueeReq -> LoginBroadcastRes)  [public, pre-auth]
 *
 * Ported verbatim from ported_3.js (handleLoginGetBroadcast).
 * reference: handle_LoginGetBroadcast @ htpp.py:7321 — one scroll marquee + one
 * broadcast message. LoginGet* endpoints are public (no token), so no account.
 */

'use strict';

function handler(reqObj, ctx) {
  return {
    scroll_res: {
      scrollMarquees: [
        { content: 'Vamos salvar Angola dessa crise...', language: 'pt-BR', region: 'BR' }
      ]
    },
    broadcast_res: {
      broadcast_messages: [
        {
          nickname: 'System',
          navigation_type: 0,
          source: 'Vamos salvar Angola dessa crise...',
          item_id: 0,
          time_stamp: Date.now(),
          source_id: 1
        }
      ],
      silence_show_switch: false
    }
  };
}

module.exports = {
  endpoint: 'LoginGetBroadcast',
  reqType: 'CSScrollMarqueeReq',
  resType: 'LoginBroadcastRes',
  handler
};
