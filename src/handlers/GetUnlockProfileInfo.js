/**
 * GetUnlockProfileInfo  (-> CSGetUnlockProfileInfoRes)
 * reference: handle_GetUnlockProfileInfo @ htpp.py:6226 — empty stub.
 *
 * Ported verbatim from ported_6.js, where it was registered via the
 * emptyStub('GetUnlockProfileInfo') factory: log the endpoint name and return a
 * default-constructed response. port_resolve: resType resolved to
 * CSGetUnlockProfileInfoRes (repeated UnlockProfileConfig infos); no matching
 * req message exists, so reqType is left null.
 */

'use strict';

function handler(reqObj, ctx) {
  ctx.logger.info('[ported_6] GetUnlockProfileInfo');
  return {};
}

module.exports = {
  endpoint: 'GetUnlockProfileInfo',
  reqType: null,
  resType: 'CSGetUnlockProfileInfoRes',
  handler
};
