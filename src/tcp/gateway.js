'use strict';

/**
 * FF 1.70 TCP gateway (notification channel, port 10300).
 *
 * The persistent, bidirectional socket the client keeps open after login so the
 * server can push messages at any time. Handshake + framing + crypto are
 * documented in protocol/TCP_PROTOCOL.md and validated against the live client.
 *
 *   client connects -> sends Cmd=1 (AES(UTF8(login_token)))
 *   server resolves the token to an account, replies single 0x02 (init ack)
 *   client heartbeats (Cmd=2) every 8s; server echoes 0x02 to keep it alive
 *   server may push at any time: push()/kick()
 */

const net = require('net');
const config = require('../../config/default');
const logger = require('../logger');
const { getRepo } = require('../db/repo');
const authStore = require('../db/authStore');
const { lookup } = require('../protocol/protos');
const { CMD, INIT_ACK, decrypt, parseClientFrames, encodeServerFrame } = require('./framing');
const tcpRouter = require('./router');
const matchmaker = require('./matchmaker');
const rooms = require('./rooms');
const { getBus } = require('../bus/instance');
const { Bus } = require('../bus');

// account uid -> { socket, account, region, lastSeen }
const sessions = new Map();

// Cross-layer PRESENCE: while a client's notification channel is connected we write
// presence:<uid> = this node to Redis (TTL-based) so OTHER processes (the HTTP tier,
// the matchmaker) can tell who is online and where. Refreshed on a heartbeat so a
// crashed gateway's keys expire; cleared on disconnect. Best-effort — a Redis blip
// never touches the socket path.
const PRESENCE_TTL_SEC = 45;
const PRESENCE_REFRESH_MS = 20000;

function markOnline(uid) {
  const bus = getBus();
  if (bus) bus.setPresence(uid, PRESENCE_TTL_SEC).catch((e) => logger.warn(`[tcp] presence set uid=${uid}: ${e.message}`));
}
function markOffline(uid) {
  const bus = getBus();
  if (bus) bus.clearPresence(uid).catch((e) => logger.warn(`[tcp] presence clear uid=${uid}: ${e.message}`));
}

// True if `account` is banned per the game store's flag or the auth store
// (auth.accounts.banned / authorized=false). Fails open on a DB error. Gated by
// config.auth.enforceBans.
async function isBanned(account) {
  if (!config.auth || !config.auth.enforceBans || !account) return false;
  if (account.banned === true) return true;
  if (!authStore.isEnabled() || !account.open_id) return false;
  try {
    const a = await authStore.accountByOpenId(account.open_id);
    return !!(a && (a.banned === true || a.authorized === false));
  } catch (e) {
    logger.warn(`[tcp] ban check uid=${account.uid}: ${e.message}`);
    return false;
  }
}

function register(entry) {
  const prev = sessions.get(entry.account.uid);
  if (prev && prev.socket !== entry.socket) {
    try { prev.socket.destroy(); } catch (e) { /* ignore */ }
  }
  sessions.set(entry.account.uid, entry);
}

function handleConnection(socket) {
  const peer = `${socket.remoteAddress}:${socket.remotePort}`;
  socket.setNoDelay(true);
  logger.info(`[tcp] connection from ${peer}`);

  let buf = Buffer.alloc(0);
  let entry = null; // set once authenticated
  // handleFrame awaits the DB on AUTH, so serialise per-connection processing to
  // a promise chain — otherwise a second 'data' chunk could interleave reads and
  // writes of buf/entry while the first is awaiting.
  let queue = Promise.resolve();

  socket.on('data', (chunk) => {
    queue = queue
      .then(async () => {
        buf = Buffer.concat([buf, chunk]);
        let parsed;
        try {
          parsed = parseClientFrames(buf);
        } catch (err) {
          logger.error(`[tcp] frame parse error from ${peer}: ${err.message}`);
          socket.destroy();
          return;
        }
        buf = parsed.rest;
        for (const f of parsed.frames) {
          try {
            entry = (await handleFrame(socket, f, entry, peer)) || entry;
          } catch (err) {
            logger.error(`[tcp] handler error (cmd=${f.cmd}) from ${peer}: ${err.stack || err}`);
          }
        }
      })
      .catch((err) => logger.error(`[tcp] data processing error ${peer}: ${err.stack || err}`));
  });

  socket.on('close', () => {
    if (entry && sessions.get(entry.account.uid) && sessions.get(entry.account.uid).socket === socket) {
      sessions.delete(entry.account.uid);
      matchmaker.cancel(entry.account.uid);
      rooms.handleDisconnect(entry.account.uid); // leave/dismiss any custom room + notify survivors
      markOffline(entry.account.uid); // clear cross-layer presence
      logger.info(`[tcp] disconnected uid=${entry.account.uid} (${peer})`);
    } else {
      logger.info(`[tcp] disconnected ${entry ? 'uid=' + entry.account.uid + ' (stale) ' : '(unauth) '}${peer}`);
    }
  });
  socket.on('error', (e) => logger.warn(`[tcp] socket error ${peer}: ${e.code}`));
}

// Returns the (new) session entry when auth happens, else undefined.
async function handleFrame(socket, f, entry, peer) {
  // Heartbeat: echo a bare 0x02 so the client's 180s liveness timer resets.
  if (f.cmd === CMD.HEARTBEAT) {
    socket.write(INIT_ACK);
    if (entry) entry.lastSeen = Date.now();
    return;
  }

  // Auth: Cmd=1 payload = AES(UTF8(session token from MajorLoginRes.token)).
  if (f.cmd === CMD.AUTH) {
    let token;
    try {
      token = decrypt(f.payload).toString('utf8');
    } catch (err) {
      logger.warn(`[tcp] ${peer}: auth payload decrypt failed -> closing`);
      socket.destroy();
      return;
    }
    const account = token ? await getRepo().getByToken(token) : null;
    if (!account) {
      logger.warn(`[tcp] ${peer}: unknown/expired token -> closing`);
      socket.destroy();
      return;
    }
    // Ban gate (defense in depth): a banned account must not hold a live
    // notification channel even if it somehow obtained a token. Checks the game
    // store's flag + the auth store; OFF/safe by default (config.auth.enforceBans
    // is on, but no account is banned until an operator flags it).
    if (await isBanned(account)) {
      logger.warn(`[tcp] ${peer}: banned account uid=${account.uid} -> kick+close`);
      try { socket.write(encodeServerFrame(CMD.KICK, Buffer.from([0]))); } catch (e) { /* ignore */ }
      setTimeout(() => { try { socket.destroy(); } catch (e) { /* ignore */ } }, 100);
      return;
    }
    const newEntry = { socket, account, region: f.region, lastSeen: Date.now() };
    register(newEntry);
    markOnline(account.uid); // publish cross-layer presence
    socket.write(INIT_ACK); // server init ack
    logger.info(`[tcp] authenticated uid=${account.uid} nickname="${account.nickname || ''}" region=${f.region} (${peer})`);
    return newEntry;
  }

  // Application message: Cmd = base protocol (tcp.EProtocol.Proto, e.g. 3 =
  // MATCHMAKING). Payload = AES(ProtoReq{cmd=subcmd, data=inner request}).
  if (!entry) {
    logger.warn(`[tcp] ${peer}: app msg cmd=${f.cmd} before auth -> ignoring`);
    return;
  }
  entry.lastSeen = Date.now();
  // f.payload is a view into the recv buffer; copy so async dispatch is safe.
  const payload = f.payload ? Buffer.from(f.payload) : null;
  handleApp(socket, entry, f.cmd, payload).catch(
    (err) => logger.error(`[tcp] ${peer}: app handling failed: ${err.stack || err}`));
}

// Decode a client app message (ProtoReq), route it, and send the MessageNotify
// response (frame cmd = the request's base protocol).
async function handleApp(socket, entry, protocol, payload) {
  let reqBytes;
  try {
    reqBytes = payload ? decrypt(payload) : Buffer.alloc(0);
  } catch (e) {
    logger.warn(`[tcp] uid=${entry.account.uid} protocol=${protocol}: payload decrypt failed`);
    return;
  }

  const ProtoReq = lookup('ProtoReq');
  let preq;
  try {
    preq = ProtoReq.toObject(ProtoReq.decode(reqBytes), { longs: Number, enums: Number, bytes: Buffer, defaults: true });
  } catch (e) {
    logger.warn(`[tcp] uid=${entry.account.uid} protocol=${protocol}: ProtoReq decode failed`);
    return;
  }

  const subcmd = preq.cmd;
  const ctx = {
    account: entry.account,
    protocol,
    subcmd,
    reqData: Buffer.isBuffer(preq.data) ? preq.data : Buffer.alloc(0),
    region: entry.region,
    logger,
    gateway: module.exports
  };

  const result = await tcpRouter.dispatch(ctx);
  if (!result) {
    logger.info(`[tcp] uid=${entry.account.uid} unrouted protocol=${protocol} subcmd=${subcmd} (${reqBytes.length}B)`);
    return;
  }
  if (result.noReply) return; // handler consumed it without a response

  let content = Buffer.alloc(0);
  if (result.resType) {
    const T = lookup(result.resType);
    if (T) content = Buffer.from(T.encode(T.fromObject(result.resObj)).finish());
  }
  const resCmd = result.resCmd != null ? result.resCmd : subcmd;
  sendNotify(socket, entry.account.uid, protocol, result.ret || 0, resCmd, content);
  logger.info(`[tcp] uid=${entry.account.uid} -> ${result.resType || 'MessageNotify'} ` +
    `(protocol=${protocol} cmd=${resCmd} ret=${result.ret || 0}, ${content.length}B)`);
}

// Wrap `content` in a MessageNotify and send it as a framed, AES-encrypted
// packet (frame cmd = protocol).
function sendNotify(socket, accountId, protocol, ret, cmd, content) {
  const MessageNotify = lookup('MessageNotify');
  const bytes = MessageNotify.encode(MessageNotify.fromObject({
    account_id: accountId, protocol, ret, cmd, content: content || Buffer.alloc(0)
  })).finish();
  // server->client payload is PLAINTEXT protobuf (client never decrypts recvs).
  socket.write(encodeServerFrame(protocol, Buffer.from(bytes)));
}

// --- server push API -------------------------------------------------------

/**
 * Send raw bytes as a frame with command `cmd`. NOTE: server->client payloads
 * are PLAINTEXT protobuf — the client's receive path (ServiceClient
 * ::HandleRecvPacket) deserializes the payload directly and never decrypts.
 * (Only client->server is AES-encrypted; we decrypt those on the way in.)
 */
function pushRaw(accountId, cmd, plaintextBuf) {
  const entry = sessions.get(accountId);
  if (!entry) return false;
  entry.socket.write(encodeServerFrame(cmd, plaintextBuf || Buffer.alloc(0)));
  return true;
}

/**
 * Send a MessageNotify to a connected account: encode `obj` as `typeName`, wrap
 * it as MessageNotify{protocol, ret, cmd, content} and frame it (plaintext).
 * Used for server-initiated pushes (e.g. the matchmaker's SussNtf). Returns
 * false if the account is not connected.
 */
function notify(accountId, protocol, cmd, typeName, obj, ret) {
  const entry = sessions.get(accountId);
  if (!entry) return false;
  let content = Buffer.alloc(0);
  if (typeName) {
    const T = lookup(typeName);
    if (T) content = Buffer.from(T.encode(T.fromObject(obj || {})).finish());
  }
  sendNotify(entry.socket, accountId, protocol, ret || 0, cmd, content);
  return true;
}

/**
 * Send a MessageNotify with PRE-ENCODED `content` bytes to a connected account.
 * Used by the cross-layer push relay (gw.push): the publisher already built the exact
 * TCP proto, so the gateway just wraps + frames it. False if not connected here.
 */
function notifyRaw(accountId, protocol, cmd, content, ret) {
  const entry = sessions.get(accountId);
  if (!entry) return false;
  sendNotify(entry.socket, accountId, protocol, ret || 0, cmd, content || Buffer.alloc(0));
  return true;
}

/** Encode a protobuf object of `typeName` and push it as command `cmd`. */
function push(accountId, cmd, typeName, obj) {
  const T = lookup(typeName);
  if (!T) throw new Error(`[tcp] push: unknown proto type '${typeName}'`);
  const payload = Buffer.from(T.encode(T.fromObject(obj || {})).finish());
  return pushRaw(accountId, cmd, payload);
}

/**
 * Kick/ban a connected account: send Cmd=11, then close. The client disconnects
 * on receipt (parsed reason, or "Unknown" if it can't parse). `reasonPayload`
 * (a Buffer) is optional; the exact kick-reason protobuf is a TODO — see
 * protocol/TCP_PROTOCOL.md.
 */
function kick(accountId, reasonPayload) {
  const entry = sessions.get(accountId);
  if (!entry) return false;
  const payload = reasonPayload && reasonPayload.length ? reasonPayload : Buffer.from([0]);
  entry.socket.write(encodeServerFrame(CMD.KICK, payload));
  logger.info(`[tcp] kick uid=${accountId}`);
  setTimeout(() => { try { entry.socket.destroy(); } catch (e) { /* ignore */ } }, 250);
  return true;
}

function isOnline(accountId) { return sessions.has(accountId); }
function onlineCount() { return sessions.size; }

// Refresh presence TTL for every locally-connected account in one pipeline, so a
// live gateway keeps its uids marked online while a crashed one lets them expire.
function startPresenceHeartbeat() {
  const t = setInterval(() => {
    const bus = getBus();
    if (!bus || !sessions.size) return;
    bus.refreshPresence([...sessions.keys()], PRESENCE_TTL_SEC).catch(() => {});
  }, PRESENCE_REFRESH_MS);
  if (t.unref) t.unref();
  return t;
}

// Subscribe to cross-layer bus events (best-effort; no-op if the bus is disabled):
//  - gw.push: another layer built a MessageNotify for a specific account -> relay it
//    if that client is connected HERE (broadcast across gateways + local filter).
//  - player.updated: a profile changed (usually via the HTTP tier) -> refresh the
//    cached account IN PLACE so the matchmaker's prepare_token carries fresh cosmetics
//    (the queue holds the same object reference, so Object.assign reaches it).
function startBusSubscriptions() {
  const bus = getBus();
  if (!bus) return;

  bus.subscribePS('gw.push', (env) => {
    const m = Bus.payload(env, 'GatewayPush');
    const target = Number(m.target_account_id);
    if (!sessions.has(target)) return; // another node owns this client (or it's offline)
    const content = Buffer.isBuffer(m.content) ? m.content : Buffer.from(m.content || []);
    notifyRaw(target, m.protocol, m.cmd, content);
    logger.info(`[tcp] relay gw.push -> uid=${target} protocol=${m.protocol} cmd=${m.cmd} (${content.length}B)`);
  });

  // session.revoke: the login/main tier (or a moderator action) asks us to kick a
  // specific account — bad signature_md5, ban, forced logout. Kick it if it's
  // connected HERE; another gateway node handles its own sessions.
  bus.subscribePS('session.revoke', (env) => {
    const m = Bus.payload(env, 'SessionRevoke');
    const uid = Number(m.account_id);
    if (!sessions.has(uid)) return;
    logger.info(`[tcp] session.revoke uid=${uid} reason="${m.reason || ''}" -> kick`);
    kick(uid);
  });

  bus.subscribePS('player.updated', async (env) => {
    const m = Bus.payload(env, 'PlayerUpdated');
    const uid = Number(m.account_id);
    const entry = sessions.get(uid);
    if (!entry) return; // not connected here
    try {
      const fresh = await getRepo().getById(uid);
      if (fresh) {
        Object.assign(entry.account, fresh); // mutate in place — the matchmaker holds this ref
        logger.info(`[tcp] refreshed cached account uid=${uid} (player.updated)`);
      }
    } catch (e) {
      logger.warn(`[tcp] player.updated refresh uid=${uid}: ${e.message}`);
    }
  });
  logger.info('[tcp] bus subscriptions up (gw.push, session.revoke, player.updated)');
}

function createGateway() {
  startPresenceHeartbeat();
  startBusSubscriptions();
  return net.createServer(handleConnection);
}

module.exports = { createGateway, notify, notifyRaw, push, pushRaw, kick, isOnline, onlineCount, sessions };
