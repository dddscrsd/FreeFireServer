/**
 * RenamePet  (CSRenamePetReq -> CSRenameRes)  [ref @7672]
 *
 * Ported from ported_1.js (handleRenamePet). Reference is an empty stub; echo
 * back the requested pet id + name.
 */

'use strict';

function handler(reqObj, ctx) {
  ctx.logger.info(`[ported_1] RenamePet pet_id=${reqObj && reqObj.pet_id}`);
  return {
    pet_id: (reqObj && reqObj.pet_id) || 0,
    name: (reqObj && reqObj.name) || ''
  };
}

module.exports = {
  endpoint: 'RenamePet',
  reqType: 'CSRenamePetReq',
  resType: 'CSRenameRes',
  handler
};
