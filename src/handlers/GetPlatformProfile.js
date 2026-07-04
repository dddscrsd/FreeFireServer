/**
 * GetPlatformProfile  (PlatformProfileReq -> PlatformProfileRes)
 *
 * Ported from login.js (handleGetPlatformProfile). Echoes the requested
 * external identity back to the client.
 */

'use strict';

function handler(reqObj) {
  return {
    external_id: reqObj.external_id || '',
    external_type: reqObj.platform_sdk_id || 0,
    external_name: '',
    external_icon: ''
  };
}

module.exports = {
  endpoint: 'GetPlatformProfile',
  reqType: 'PlatformProfileReq',
  resType: 'PlatformProfileRes',
  handler
};
