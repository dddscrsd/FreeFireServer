/**
 * LinkFacebookAccount  (-> Empty)  [ref @5914]
 *
 * Reference is an empty stub; no dedicated res type exists, resolved to Empty.
 */

'use strict';

function handleLinkFacebookAccount(reqObj, ctx) {
  ctx.logger.info('[ported_1] LinkFacebookAccount');
  return {};
}

module.exports = {
  endpoint: 'LinkFacebookAccount',
  reqType: null,
  resType: 'Empty',
  handler: handleLinkFacebookAccount
};
