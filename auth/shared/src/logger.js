'use strict';

// IMPORTANT: never log tokens, Discord tokens, session secrets, or any
// configuration secret. Log identifiers (uid, dc_id, open_id) and route names
// only. Anything sensitive is passed through this file only when it has been
// explicitly redacted or truncated to the point of uselessness.

const pino = require('pino');
const config = require('./config');

const transport = config.isProduction
  ? undefined
  : {
      target: 'pino-pretty',
      options: {
        colorize: true,
        singleLine: true,
        translateTime: 'SYS:standard',
      },
    };

const logger = pino({
  level: config.LOG_LEVEL,
  base: null,
  transport,
});

function childForRequest(baseLogger, req, { service, reqId }) {
  return baseLogger.child({
    service,
    reqId,
    method: req.method,
    route: req.originalUrl?.split('?')[0] ?? req.path,
  });
}

module.exports = { logger, childForRequest };
