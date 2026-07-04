/**
 * GetMailList (CSGetMailListReq -> CSGetMailListRes). Reference @3207 returns an
 * empty body; we surface any mails stored on the account (none by default).
 *
 * Ported verbatim from ported_2.js (handleGetMailList).
 */

'use strict';

const { requireAccount } = require('./_shared');

function handleGetMailList(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const mails = (account.mails || []).map((m) => ({
    mail_id: m.mail_id || 0,
    type: m.type || 0,
    title: m.title || '',
    content: m.content || '',
    receive_time: m.receive_time || 0,
    status: m.status || 0,
    source: m.source || 0,
    action_type: m.action_type || 0
  }));
  return { mails };
}

module.exports = {
  endpoint: 'GetMailList',
  reqType: 'CSGetMailListReq',
  resType: 'CSGetMailListRes',
  handler: handleGetMailList
};
