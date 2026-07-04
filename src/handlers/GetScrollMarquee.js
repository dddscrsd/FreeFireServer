/**
 * GetScrollMarquee  (CSScrollMarqueeReq -> CSScrollMarqueeRes)  [types resolved]
 *
 * Ported from ported_0.js (handleGetScrollMarquee).
 * reference: handle_GetScrollMarquee @ htpp.py:6221 — empty stub. Not present in
 * endpoint_map.json, so types are supplied explicitly.
 */

'use strict';

function handler() {
  return { scrollMarquees: [] };
}

module.exports = {
  endpoint: 'GetScrollMarquee',
  reqType: 'CSScrollMarqueeReq',
  resType: 'CSScrollMarqueeRes',
  handler
};
