'use strict';

const crypto = require('crypto');
const express = require('express');
const {
  config,
  logger,
  childForRequest,
  createSecurityHeaders,
} = require('@auth/shared');

const path = require('path');
const registerMiscRoutes = require('./routes/misc');
const registerDialogRoutes = require('./routes/dialog');
const registerDiscordRoutes = require('./routes/discord');
const registerLegalRoutes = require('./routes/legal');
const registerPairRoutes = require('./routes/pair');
const renderErrorPage = require('./views/error');

const STATIC_DIR = path.join(__dirname, '..', 'static');

const SERVICE = 'login-server';

// CSP applied to every response. HTML pages carry inline styles, so
// style-src 'unsafe-inline' is required. There is no inline script anywhere
// in the login flow (the redirect page uses <meta http-equiv="refresh">).
const CSP_HEADER = [
  "default-src 'self'",
  "base-uri 'self'",
  "frame-ancestors 'none'",
  "img-src 'self' data:",
  "style-src 'self' 'unsafe-inline'",
  "font-src 'self'",
  "script-src 'self'",
  "form-action 'self'",
  "object-src 'none'",
].join('; ');

function createApp({ guests, accounts, pairings }) {
  const app = express();
  app.disable('x-powered-by');
  app.set('trust proxy', config.TRUST_PROXY);

  app.use(express.urlencoded({ extended: false }));
  app.use(express.json());
  app.use(createSecurityHeaders({ isProduction: config.isProduction }));
  app.use((req, res, next) => {
    res.setHeader('Content-Security-Policy', CSP_HEADER);
    next();
  });

  // Request context.
  app.use((req, res, next) => {
    const reqId = req.get('X-Request-Id') || crypto.randomUUID();
    req.reqId = reqId;
    req.log = childForRequest(logger, req, { service: SERVICE, reqId });
    res.setHeader('X-Request-Id', reqId);
    next();
  });

  // Serve the polling script under /static/. Same-origin, so the strict
  // CSP applied above (script-src 'self') allows the WebView to load it.
  app.use('/static', express.static(STATIC_DIR, {
    maxAge: '1h',
    fallthrough: false,
    index: false,
  }));

  registerMiscRoutes(app);
  registerLegalRoutes(app);
  registerPairRoutes(app, { pairings });
  registerDialogRoutes(app, { pairings });
  registerDiscordRoutes(app, { guests, accounts, pairings });

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
    // If the client asked for JSON, respond in kind; otherwise render the
    // generic error page.
    const acceptsHtml = (req.get('Accept') || '').includes('text/html');
    if (acceptsHtml) {
      return res.status(500).type('html').send(renderErrorPage({
        title: 'Something went wrong',
        message: 'An unexpected error occurred. Please try again.',
      }));
    }
    res.status(500).json({ error: 'internal_error' });
  });

  return app;
}

module.exports = createApp;
