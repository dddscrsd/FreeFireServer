const logger = require('../logger');

// Safe JSON for log lines: render Buffers compactly, tolerate BigInt, and cap
// the length so a huge message can't flood the log.
function safeJson(value, max = 4000) {
  let out;
  try {
    out = JSON.stringify(value, (key, v) => {
      if (Buffer.isBuffer(v)) return `<Buffer ${v.length}B>`;
      if (v && v.type === 'Buffer' && Array.isArray(v.data)) return `<Buffer ${v.data.length}B>`;
      if (typeof v === 'bigint') return v.toString();
      return v;
    });
  } catch (err) {
    return `<unserialisable: ${err.message}>`;
  }
  if (out && out.length > max) out = out.slice(0, max) + `…(+${out.length - max})`;
  return out;
}

module.exports = function applyRequestLogger(app) {
    app.use((req, res, next) => {
        const start = performance.now();

        res.on('finish', () => {
            const elapsed = (performance.now() - start).toFixed(2);

            let msg =
                `[HTTP] ${req.method.padEnd(6)} ` +
                `${req.originalUrl} -> ${res.statusCode} (${elapsed}ms)`;

            // Protobuf game routes: the router AES-decrypts + protobuf-decodes the
            // body and attaches the decoded request/response objects. Log THOSE
            // as JSON — never the raw encrypted Buffer (`req.body`).
            if (req.decodedRequest !== undefined) {
                const t = req.reqTypeName ? ` (${req.reqTypeName})` : '';
                msg += ` | req${t}=${safeJson(req.decodedRequest)}`;
                if (req.decodedResponse !== undefined) {
                    msg += ` | res=${safeJson(req.decodedResponse)}`;
                }
            } else if (
                req.body &&
                !Buffer.isBuffer(req.body) &&
                typeof req.body === 'object' &&
                Object.keys(req.body).length > 0 &&
                ['POST', 'PUT', 'PATCH'].includes(req.method)
            ) {
                // Non-protobuf JSON routes (the legacy Express routes).
                msg += ` | body=${safeJson(req.body)}`;
            }

            logger.info(msg);
        });

        next();
    });
};
