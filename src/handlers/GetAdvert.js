/**
 * GetAdvert  (CSGetAdvertReq -> CSGetAdvertRes)  [resolved from null in manifest]
 *
 * Ported verbatim from ported_4.js (handleGetAdvert).
 * reference get_advert @ htpp.py:4449 returns empty body (no adverts).
 */

'use strict';

function handler() {
  return { advert_items: [] };
}

module.exports = {
  endpoint: 'GetAdvert',
  reqType: 'CSGetAdvertReq',
  resType: 'CSGetAdvertRes',
  handler
};
