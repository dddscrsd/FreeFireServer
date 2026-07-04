/**
 * GetPresenceByAccountIds  (Empty -> Empty)  [resolved from null in manifest]
 *
 * Ported from ported_4.js (handleGetPresenceByAccountIds). reference @
 * htpp.py:5934 returns empty body; no HTTP res message exists, so we resolve to
 * Empty.
 */

'use strict';

function handleGetPresenceByAccountIds() {
  return {};
}

module.exports = {
  endpoint: 'GetPresenceByAccountIds',
  reqType: 'Empty',
  resType: 'Empty',
  handler: handleGetPresenceByAccountIds
};
