/**
 * LoginGetDesc  (CSLoginDescReq -> LoginDescRes)  [public, LoginGet* handshake]
 *
 * Returns the login-description blob the client needs to finish loading into the
 * lobby (adverts, switches, activity/gacha/ranking/store configs, ...).
 *
 * Previously this shipped the pre-serialized protocol/LoginDescRes.bin verbatim
 * (~130 KB, generated from original_to_read/gen_logindescres.py). That gave no
 * control over events. It now returns an editable plain object from
 * _loginDescData.js — trimmed to one representative entry per repeated field —
 * and lets the router encode it against LoginDescRes. Add/remove entries in the
 * data module to control adverts, activities, gacha banners, store tabs, etc.
 */

'use strict';

const loginDesc = require('./_loginDescData');

function handler(reqObj, ctx) {
  ctx.logger.info('[login] LoginGetDesc -> LoginDescRes (from _loginDescData)');
  return loginDesc;
}

module.exports = {
  endpoint: 'LoginGetDesc',
  reqType: 'CSLoginDescReq',
  resType: 'LoginDescRes',
  handler
};
