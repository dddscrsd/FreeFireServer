'use strict';

const {
  config,
  logger,
  generateOpenId,
  nextUid,
  withUidRetry,
  issueTokens,
  buildSignedRequest,
  nowSeconds,
} = require('@auth/shared');

// URL-encoded scope list preserved verbatim from the original Flask server
// so the game client's SDK sees the exact same fragment format it expects.
const GRANTED_SCOPES = 'user_friends%2Cemail%2Copenid%2Cpublic_profile%2Cbasic_info';

// Original data_access_expiration_time was create_time + 5183716 (~60 days).
const DATA_ACCESS_OFFSET_SECONDS = 5_183_716;

// completeDiscordLogin
//
// Given a fetched Discord user, either provisions a fresh guests/accounts
// pair or refreshes the Discord fields on an existing pair, then applies
// the renovation gate, issues fresh tokens, builds the signed_request, and
// returns the fbconnect://... redirect URL that the browser will navigate
// to.
//
// Returns:
//   { ok: true,  redirectUrl, uid, openId }
//   { ok: false, reason: 'renovation' }
async function completeDiscordLogin({ discordUser, redirectUri, ip, guests, accounts, log }) {
  const flowLog = log || logger;
  const dcId = discordUser.id;
  const now = nowSeconds();
  const nowIso = new Date(now * 1000).toISOString();

  const existing = await guests.findByDcId(dcId);

  let user;
  if (existing) {
    // Update Discord fields in case handle/email changed since last login.
    await guests.updateDiscordFields(existing.open_id, {
      dc_handle: discordUser.username,
      dc_email: discordUser.email,
    });

    if (existing.renovation === true) {
      flowLog.warn({ dc_id: dcId, uid: existing.uid }, 'login blocked by renovation flag');
      return { ok: false, reason: 'renovation' };
    }

    user = { ...existing, dc_handle: discordUser.username, dc_email: discordUser.email };
    flowLog.info({ dc_id: dcId, uid: user.uid }, 'returning user signed in');
  } else {
    user = await withUidRetry(async () => {
      const openId = generateOpenId();
      const uid = await nextUid(guests);

      const guestDoc = {
        open_id: openId,
        uid,
        dc_id: dcId,
        dc_handle: discordUser.username,
        dc_email: discordUser.email,
        renovation: false,
        access_token: null,
        refresh_token: null,
        access_expiry: null,
        refresh_expiry: null,
        create_time: null,
        last_login: null,
        last_ip: null,
        registered_at: nowIso,
        registered_ip: ip,
        renovated_at: null,
        renovated_ip: null,
      };

      const accountDoc = {
        open_id: openId,
        uid,
        nickname: null,
        banned: false,
        ban_reason: null,
        authorized: true,
        under_analysis: false,
        created_at: nowIso,
      };

      // Insert guests first; its dc_id uniqueness index is what protects
      // against concurrent-provisioning races for the same Discord user.
      await guests.insertUser(guestDoc);
      try {
        await accounts.insertAccount(accountDoc);
      } catch (err) {
        // If accounts insert fails after a successful guests insert, we
        // leave a guests row without an accounts row. Log loudly so an
        // operator can clean up manually; the next login for the same
        // dc_id will otherwise reuse the guest row and skip account
        // creation, leaving the state permanently inconsistent.
        flowLog.error(
          { err, open_id: openId, uid, dc_id: dcId },
          'accounts insert failed after guests insert; orphaned guest row'
        );
        throw err;
      }

      flowLog.info({ dc_id: dcId, uid, open_id: openId }, 'new user provisioned');
      return guestDoc;
    });
  }

  const tokenSet = await issueTokens(guests, user.open_id, { ip, now });

  const signedRequest = buildSignedRequest({
    uid: user.uid,
    accessToken: tokenSet.access_token,
    appId: config.APP_ID,
    appSecret: config.APP_SECRET,
    issuedAt: tokenSet.create_time,
  });

  const dataAccessExpiry = tokenSet.create_time + DATA_ACCESS_OFFSET_SECONDS;
  const redirectUrl =
    `${redirectUri}#granted_scopes=${GRANTED_SCOPES}` +
    `&denied_scopes=` +
    `&signed_request=${signedRequest}` +
    `&access_token=${tokenSet.access_token}` +
    `&data_access_expiration_time=${dataAccessExpiry}` +
    `&expires_in=1296000`;

  return { ok: true, redirectUrl, uid: user.uid, openId: user.open_id };
}

module.exports = { completeDiscordLogin };
