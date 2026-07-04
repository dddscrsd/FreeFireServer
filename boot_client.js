/**
 * Boot-sequence smoke client: simulates the real game client boot flow against
 * the local protobuf server. Each step is an AES(protobuf) POST to /<Endpoint>;
 * authed steps carry the Bearer token from the MajorLogin response.
 */
'use strict';

const http = require('http');
const { encrypt, decrypt } = require('./src/protocol/aes');
const { lookup } = require('./src/protocol/protos');

const HOST = '127.0.0.1';
const PORT = 3000;

function post(endpoint, cipher, token) {
  return new Promise((resolve, reject) => {
    const headers = { 'Content-Type': 'application/octet-stream', 'Content-Length': cipher.length };
    if (token) headers['Authorization'] = `Bearer ${token}`;
    const req = http.request({ host: HOST, port: PORT, method: 'POST', path: `/${endpoint}`, headers }, (res) => {
      const chunks = [];
      res.on('data', (c) => chunks.push(c));
      res.on('end', () => resolve({ status: res.statusCode, body: Buffer.concat(chunks) }));
    });
    req.on('error', reject);
    req.write(cipher);
    req.end();
  });
}

async function call(endpoint, reqTypeName, reqObj, resTypeName, token) {
  const ReqType = lookup(reqTypeName);
  if (!ReqType) throw new Error(`reqType not found: ${reqTypeName}`);
  const msg = ReqType.fromObject(reqObj || {});
  const plain = Buffer.from(ReqType.encode(msg).finish());
  const cipher = encrypt(plain);

  const { status, body } = await post(endpoint, cipher, token);
  let decoded = null;
  let err = null;
  try {
    const plainRes = body; // responses are plaintext protobuf (only requests are AES-encrypted)
    const ResType = lookup(resTypeName) || lookup('Empty');
    const m = ResType.decode(plainRes);
    decoded = ResType.toObject(m, { longs: Number, enums: Number, defaults: true, arrays: true, objects: true });
  } catch (e) {
    err = e.message;
  }
  return { status, decoded, err, rawLen: body.length };
}

function pick(obj, keys) {
  const out = {};
  if (!obj) return out;
  for (const k of keys) if (obj[k] !== undefined) out[k] = obj[k];
  return out;
}

(async () => {
  console.log('=== BOOT SEQUENCE ===\n');

  // 1) MajorLogin (public)
  const login = await call('MajorLogin', 'LoginReq', {
    open_id: 'boot-test-002',
    open_id_type: 1,
    nickname: 'BootTester',
    device_id: 'dev-boot-001',
    client_version: '1.70.4'
  }, 'MajorLoginRes', null);
  console.log('MajorLogin   status=%d len=%d err=%s', login.status, login.rawLen, login.err);
  console.log('  ->', JSON.stringify(pick(login.decoded, ['account_id', 'token', 'ttl', 'server_url', 'ip_region', 'recommend_regions'])));
  const token = login.decoded && login.decoded.token;
  if (!token) { console.log('NO TOKEN - aborting'); process.exit(1); }

  // 2) GetLoginData (authed)
  const ld = await call('GetLoginData', 'LoginReq', { account_id: login.decoded.account_id }, 'LoginRes', token);
  console.log('\nGetLoginData status=%d len=%d err=%s', ld.status, ld.rawLen, ld.err);
  console.log('  ->', JSON.stringify(pick(ld.decoded, ['account_id', 'nickname', 'level', 'exp', 'coins', 'gems', 'region', 'clan_id', 'create_at'])));

  // 3) GetProfiles (authed)
  const gp = await call('GetProfiles', 'AvatarProfile', {}, 'CSGetProfileListRes', token);
  const profs = (gp.decoded && gp.decoded.profiles) || [];
  console.log('\nGetProfiles  status=%d len=%d err=%s', gp.status, gp.rawLen, gp.err);
  console.log('  -> profiles=%d selected=%s', profs.length, JSON.stringify(profs.filter(p => p.is_selected).map(p => p.avatar_id)));

  // 4) GetWallet (authed)
  const gw = await call('GetWallet', 'CSGetWalletReq', {}, 'CSGetWalletRes', token);
  console.log('\nGetWallet    status=%d len=%d err=%s', gw.status, gw.rawLen, gw.err);
  console.log('  ->', JSON.stringify(pick(gw.decoded, ['account_id', 'wallet'])));

  // 5) GetBackpack (authed)
  const gb = await call('GetBackpack', 'CSGetBackpackReq', { is_login: true }, 'CSGetBackpackRes', token);
  console.log('\nGetBackpack  status=%d len=%d err=%s', gb.status, gb.rawLen, gb.err);
  console.log('  -> wallet=%s items=%d selected_avatar=%s',
    JSON.stringify(gb.decoded && gb.decoded.wallet),
    ((gb.decoded && gb.decoded.items) || []).length,
    gb.decoded && gb.decoded.selected_items && gb.decoded.selected_items.avatar_id);

  // 6) GetMailList (authed)
  const gm = await call('GetMailList', 'CSGetMailListReq', { language: 'en' }, 'CSGetMailListRes', token);
  console.log('\nGetMailList  status=%d len=%d err=%s', gm.status, gm.rawLen, gm.err);
  console.log('  -> mails=%d', ((gm.decoded && gm.decoded.mails) || []).length);

  console.log('\n=== DONE ===');
  console.log('TOKEN=' + token);
  console.log('ACCOUNT_ID=' + login.decoded.account_id);
})().catch((e) => { console.error('CLIENT ERROR:', e); process.exit(1); });
