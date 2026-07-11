'use strict';

const crypto = require('crypto');
const express = require('express');
const {
  config,
  logger,
  childForRequest,
  createSecurityHeaders,
} = require('@auth/shared');

const registerMiscRoutes = require('./routes/misc');
const registerOAuthRoutes = require('./routes/oauth');
const registerGraphRoutes = require('./routes/graph');

const SERVICE = 'connect-server';

function createApp({ guests }) {
  const app = express();
  app.disable('x-powered-by');
  app.set('trust proxy', config.TRUST_PROXY);

  app.use(express.urlencoded({ extended: false }));
  app.use(express.json());
  app.use(createSecurityHeaders({ isProduction: config.isProduction }));

  // Request context: correlation id and child logger.
  app.use((req, res, next) => {
    const reqId = req.get('X-Request-Id') || crypto.randomUUID();
    req.reqId = reqId;
    req.log = childForRequest(logger, req, { service: SERVICE, reqId });
    res.setHeader('X-Request-Id', reqId);
    next();
  });

  // Order matters: /health and /app/info/get are literal paths and must be
  // registered BEFORE the /:app_id catch-all inside graph routes.
  registerMiscRoutes(app);
  registerOAuthRoutes(app, { guests });
  registerGraphRoutes(app, { guests });

  // 404 fallback.
  app.use((req, res) => {
    res.status(404).json({ error: 'not_found' });
  });

  // Global error handler.
  // eslint-disable-next-line no-unused-vars
  app.use((err, req, res, next) => {
    const log = req.log || logger;
    log.error({ err }, 'unhandled request error');
    if (res.headersSent) return;
    res.status(500).json({ error: 'internal_error' });
  });

  return app;
}

module.exports = createApp;
