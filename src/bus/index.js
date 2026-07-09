// Node client for the Redis event bus: durable Streams for commands, PubSub for
// ephemeral signals, and uid->node presence keys. Mirrors match-server/bus/bus.go
// over the same bus.Envelope contract. Binary protobuf rides in Redis as raw
// bytes, so reads use ioredis Buffer-variant commands (xreadgroupBuffer /
// messageBuffer) to avoid UTF-8 corruption.
const Redis = require('ioredis');

const { Envelope, encode, decode } = require('./proto');
const logger = require('../logger');

const STREAM_MAXLEN = 100000;
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

class Bus {
  /** @param {{url:string, source:string, node?:string}} opts */
  constructor({ url, source, node }) {
    if (!url) throw new Error('bus: url (REDIS_URL) is required');
    if (!source) throw new Error('bus: source (this layer name) is required');
    this.url = url;
    this.source = source;
    this.node = node || source;
    this._conns = [];
    this.pub = this._connect(); // shared connection for publish / presence
  }

  _connect() {
    const conn = new Redis(this.url);
    conn.on('error', (err) => logger.error(`[bus] redis: ${err.message}`));
    this._conns.push(conn);
    return conn;
  }

  _wrap(type, payloadType, obj) {
    const payload = encode(payloadType, obj);
    return Buffer.from(
      Envelope.encode(
        Envelope.create({ type, source: this.source, correlation_id: '', ts_unix_ms: Date.now(), payload })
      ).finish()
    );
  }

  /** Publish a DURABLE event to stream:<type> (XADD). */
  publish(type, payloadType, obj) {
    return this.pub.xadd('stream:' + type, 'MAXLEN', '~', STREAM_MAXLEN, '*', 'env', this._wrap(type, payloadType, obj));
  }

  /** Publish an EPHEMERAL event to ps:<type> (PUBLISH). */
  publishPS(type, payloadType, obj) {
    return this.pub.publish('ps:' + type, this._wrap(type, payloadType, obj));
  }

  /**
   * Join <group> on stream:<type>. handler(env) runs per message; the message is
   * XACKed on success and left PENDING (for redelivery) if handler throws, so
   * handlers must be idempotent. Uses a dedicated blocking connection.
   */
  subscribeStream(type, group, consumer, handler) {
    const stream = 'stream:' + type;
    const conn = this._connect();
    conn.xgroup('CREATE', stream, group, '0', 'MKSTREAM').catch((err) => {
      if (!String(err.message || err).includes('BUSYGROUP')) logger.warn(`[bus] group ${stream}/${group}: ${err.message}`);
    });
    (async () => {
      for (;;) {
        if (conn.status === 'end') return;
        let res;
        try {
          res = await conn.xreadgroupBuffer('GROUP', group, consumer, 'COUNT', 32, 'BLOCK', 5000, 'STREAMS', stream, '>');
        } catch (err) {
          if (conn.status === 'end') return;
          logger.error(`[bus] xreadgroup ${stream}: ${err.message}`);
          await sleep(1000);
          continue;
        }
        if (!res) continue;
        for (const [, messages] of res) {
          for (const [idBuf, fields] of messages) {
            const id = idBuf.toString();
            const env = this._envFromFields(fields, stream);
            if (!env) {
              await conn.xack(stream, group, id);
              continue;
            }
            try {
              await handler(env);
              await conn.xack(stream, group, id);
            } catch (err) {
              logger.error(`[bus] handler ${env.type}: ${err.message} (left pending)`);
            }
          }
        }
      }
    })();
    return conn;
  }

  /** Run handler(env) for each ephemeral message on ps:<type>. Lossy by nature. */
  subscribePS(type, handler) {
    const conn = this._connect();
    conn.subscribe('ps:' + type).catch((err) => logger.error(`[bus] subscribe ${type}: ${err.message}`));
    conn.on('messageBuffer', (_channel, message) => {
      let env;
      try {
        env = Envelope.decode(message);
      } catch (err) {
        logger.error(`[bus] bad ps envelope ${type}: ${err.message}`);
        return;
      }
      Promise.resolve(handler(env)).catch((err) => logger.error(`[bus] ps handler ${type}: ${err.message}`));
    });
    return conn;
  }

  _envFromFields(fields, stream) {
    for (let i = 0; i + 1 < fields.length; i += 2) {
      if (fields[i].toString() === 'env') {
        try {
          return Envelope.decode(fields[i + 1]);
        } catch (err) {
          logger.error(`[bus] bad envelope on ${stream}: ${err.message}`);
          return null;
        }
      }
    }
    return null;
  }

  // presence: uid -> this node, with a TTL any layer can read to route a push
  setPresence(accountId, ttlSec) {
    return this.pub.set(`presence:${accountId}`, this.node, 'EX', ttlSec);
  }

  clearPresence(accountId) {
    return this.pub.del(`presence:${accountId}`);
  }

  getNode(accountId) {
    return this.pub.get(`presence:${accountId}`);
  }

  /** Batch presence lookup — returns { accountId: node } for the ONLINE ids only. */
  async getNodes(accountIds) {
    if (!accountIds || !accountIds.length) return {};
    const vals = await this.pub.mget(accountIds.map((id) => `presence:${id}`));
    const out = {};
    accountIds.forEach((id, i) => { if (vals[i]) out[id] = vals[i]; });
    return out;
  }

  /** Refresh the presence TTL for many accounts in one round-trip (pipeline). */
  async refreshPresence(accountIds, ttlSec) {
    if (!accountIds || !accountIds.length) return;
    const pipe = this.pub.pipeline();
    for (const id of accountIds) pipe.set(`presence:${id}`, this.node, 'EX', ttlSec);
    await pipe.exec();
  }

  /** Decode an envelope's inner payload into a plain object of the given type. */
  static payload(env, payloadType) {
    return decode(payloadType, env.payload);
  }

  async close() {
    await Promise.allSettled(this._conns.map((c) => c.quit().catch(() => c.disconnect())));
  }
}

module.exports = { Bus, Envelope, encode, decode };
