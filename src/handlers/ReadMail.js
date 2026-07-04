/**
 * ReadMail  (CSReadMailReq -> CSReadMailRes)
 *
 * Ported from ported_7.js (handleReadMail). reference: read_mail @ htpp.py:3211.
 * Mark the listed mails with the requested status on the player document,
 * persist, and return an (empty) CSReadMailRes (no rewards attached by the
 * reference).
 */

'use strict';

const { requireAccount } = require('./_shared');

function handler(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};

  const mailIds = Array.isArray(reqObj.mail_ids) ? reqObj.mail_ids.map((x) => Number(x)) : [];
  const status = reqObj.status != null ? reqObj.status : 1;
  const idSet = new Set(mailIds);

  const mails = Array.isArray(account.mails) ? account.mails : [];
  let changed = false;
  for (const mail of mails) {
    if (idSet.has(Number(mail.mail_id))) {
      mail.status = status;
      changed = true;
    }
  }
  if (changed) ctx.savePlayer();

  ctx.logger.info(`[ported_7] ReadMail uid=${account.uid} ids=${mailIds.length} status=${status}`);
  return {}; // CSReadMailRes: { rewards, exchangedAwards } -- none granted.
}

module.exports = {
  endpoint: 'ReadMail',
  reqType: 'CSReadMailReq',
  resType: 'CSReadMailRes',
  handler
};
