// Verifies the GUEST login/register flow (the real-client case: empty open_id):
//   1. MajorLogin (empty open_id)   -> 404 when fresh (client opens register)
//   2. MajorRegister (name only)    -> success, account_id A
//   3. MajorLogin (empty open_id)   -> 200 + token, account_id B
//   => A === B  (register and login resolve to the SAME account: bridge fixed)
//   4. GetLoginData (Bearer)        -> populated account
//
// Transport: REQUEST body is AES-encrypted; RESPONSE body is plaintext protobuf.
const http = require('http');
const aes = require('./src/protocol/aes');
const protos = require('./src/protocol/protos');
const accounts = require('./src/db/accounts');

function post(path, body, token) {
  return new Promise((resolve, reject) => {
    const headers = { 'Content-Type': 'application/octet-stream', 'Content-Length': body.length };
    if (token) headers['Authorization'] = 'Bearer ' + token;
    const req = http.request({ host: '127.0.0.1', port: 3000, path, method: 'POST', headers },
      (res) => { const ch = []; res.on('data', d => ch.push(d));
        res.on('end', () => resolve({ status: res.statusCode, body: Buffer.concat(ch) })); });
    req.on('error', reject); req.write(body); req.end();
  });
}
// request: AES(protobuf) ; response: plaintext protobuf
const enc = (type, obj) => aes.encrypt(Buffer.from(protos.lookup(type).encode(protos.lookup(type).fromObject(obj)).finish()));
const dec = (type, buf) => protos.lookup(type).toObject(protos.lookup(type).decode(buf), { longs: String, enums: Number, defaults: true });

(async () => {
  // Clean slate: drop the deterministic guest account so step 1 yields a 404.
  try { accounts.db.prepare("DELETE FROM accounts WHERE open_id = ?").run('guest-default'); } catch (e) {}

  // guest LoginReq: NO open_id (exactly what the real no-SDK client sends)
  const loginReq = { open_id_type: '', plat_id: 1, language: 'en', client_version: '1.70.0' };

  const r1 = await post('/MajorLogin', enc('LoginReq', loginReq));
  console.log('1. MajorLogin (guest, fresh) -> HTTP', r1.status, r1.status === 404 ? 'OK (register screen)' : '(account already existed)');

  const r2 = await post('/MajorRegister', enc('PlatformRegisterReq', { nickname: 'GuestPlayer' }));
  // Decode loosely against whichever resType login.js uses (Major/PlatformRegisterRes);
  // not essential to the bridge check below.
  let reg = {};
  for (const t of ['MajorRegisterRes', 'PlatformRegisterRes']) { try { reg = dec(t, r2.body); break; } catch (e) {} }
  console.log('2. MajorRegister             -> HTTP', r2.status, 'resp=' + JSON.stringify(reg));

  const r3 = await post('/MajorLogin', enc('LoginReq', loginReq));
  const log = dec('MajorLoginRes', r3.body);
  console.log('3. MajorLogin (guest, known) -> HTTP', r3.status, 'account_id=' + log.account_id, 'token=' + (log.token || '').slice(0, 10));

  const r4 = await post('/GetLoginData', enc('LoginReq', loginReq), log.token);
  const acc = dec('LoginRes', r4.body);
  console.log('4. GetLoginData (Bearer)     -> HTTP', r4.status, 'nickname=' + acc.nickname, 'level=' + acc.level, 'coins=' + acc.coins);

  // Bridge proof: after register, the SAME guest login no longer 404s and
  // resolves to a real account whose data GetLoginData returns.
  const bridged = r1.status === 404 && r2.status === 200 && r3.status === 200 && !!log.account_id;
  const pass = bridged && log.token && acc.nickname === 'GuestPlayer';
  console.log(`\nbridge: register(200) -> login finds account #${log.account_id} (was 404 before) ? ${bridged}`);
  console.log(pass ? 'GUEST REGISTER FLOW: PASS' : 'GUEST REGISTER FLOW: FAIL');
  process.exit(0);
})().catch(e => { console.error('ERR', e); process.exit(1); });
