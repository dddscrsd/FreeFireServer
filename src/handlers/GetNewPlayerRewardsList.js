/**
 * GetNewPlayerRewardsList  (Empty -> Empty)
 *
 * Ported verbatim from ported_5.js (emptyStub registration). Not in
 * endpoint_map.json (port_resolve, reference returns empty bytes) -> resolved to
 * Empty req/res. The reference handler @ htpp.py:5954 is an empty stub; we return
 * a default-constructed resType.
 */

'use strict';

function handler() {
  return {};
}

module.exports = {
  endpoint: 'GetNewPlayerRewardsList',
  reqType: 'Empty',
  resType: 'Empty',
  handler
};
