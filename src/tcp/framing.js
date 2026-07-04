'use strict';

/**
 * FF 1.70 TCP gateway framing + crypto. See protocol/TCP_PROTOCOL.md.
 *
 * Static AES-128-CBC key/IV from GCommon::TCPSession::CreateAes, confirmed
 * against the live 1.70.1 client. Only the frame PAYLOAD is encrypted.
 *
 * Wire framing is ASYMMETRIC:
 *   client -> server:  Cmd(1) Region(1) [ if Cmd!=2: Len(4 BE) Payload ]
 *   server -> client:  Cmd(1)          [ if Cmd!=2: Len(4 BE) Payload ]
 *   Cmd==2 is a bare 1-byte heartbeat/ack (no region/len/payload) in both
 *   directions except the client always emits its trailing Region byte.
 */

const crypto = require('crypto');

const KEY = Buffer.from('1860A234CF6F8B853DF690344BE791DD', 'hex');
const IV = Buffer.from('02F5989EC026B2DFB24421AD1A9F908D', 'hex');

// Reserved command bytes.
const CMD = { AUTH: 1, HEARTBEAT: 2, KICK: 11 };

// Single-byte control frame the server sends: "server init ack" / heartbeat ack.
const INIT_ACK = Buffer.from([CMD.HEARTBEAT]);

function encrypt(plain) {
  const c = crypto.createCipheriv('aes-128-cbc', KEY, IV);
  return Buffer.concat([c.update(plain), c.final()]);
}

function decrypt(cipher) {
  const d = crypto.createDecipheriv('aes-128-cbc', KEY, IV);
  return Buffer.concat([d.update(cipher), d.final()]);
}

/**
 * Parse as many complete client->server frames as are buffered.
 * Frame: Cmd(1) Region(1) [ if Cmd!=2: Len(4 BE) Payload(Len) ].
 * @returns {{frames: Array<{cmd:number,region:number,payload:Buffer|null}>, rest: Buffer}}
 */
function parseClientFrames(buf) {
  const frames = [];
  let off = 0;
  while (buf.length - off >= 2) {
    const cmd = buf[off];
    const region = buf[off + 1];
    if (cmd === CMD.HEARTBEAT) {
      frames.push({ cmd, region, payload: null });
      off += 2;
      continue;
    }
    if (buf.length - off < 6) break; // need Cmd+Region+Len
    const len = buf.readUInt32BE(off + 2);
    if (buf.length - off < 6 + len) break; // wait for the full payload
    frames.push({ cmd, region, payload: buf.subarray(off + 6, off + 6 + len) });
    off += 6 + len;
  }
  return { frames, rest: buf.subarray(off) };
}

/**
 * Build a server->client frame. `payload` must already be encrypted (or omitted).
 * Cmd==2 collapses to a single 0x02 byte.
 */
function encodeServerFrame(cmd, payload) {
  if (cmd === CMD.HEARTBEAT) return Buffer.from([CMD.HEARTBEAT]);
  const p = payload || Buffer.alloc(0);
  const head = Buffer.alloc(5);
  head[0] = cmd;
  head.writeUInt32BE(p.length, 1);
  return Buffer.concat([head, p]);
}

module.exports = {
  KEY,
  IV,
  CMD,
  INIT_ACK,
  encrypt,
  decrypt,
  parseClientFrames,
  encodeServerFrame
};
