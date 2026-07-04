const express = require('express');
require('express-async-errors');

const applyRequestLogger = require('../middleware/requestLogger');
const errorHandler = require('../middleware/errorHandler');

// Common setup shared by all three split apps. The game client is a plain-HTTP
// binary protocol client, not a browser, so strip Express niceties that only
// confuse it / waste bytes.
function createBaseApp() {
  const app = express();
  app.disable('x-powered-by');
  app.set('etag', false);
  // Logging (registers a res.on('finish') listener; must run before the routes).
  applyRequestLogger(app);
  return app;
}

// Register a liveness route, then the 404 + error handler. Call AFTER routes are
// mounted so the 404 is the last matcher and errorHandler is truly last.
function finalizeApp(app, name) {
  app.get('/health', (req, res) => res.json({ status: 'ok', server: name }));
  app.use((req, res) => {
    res.type('text/plain');
    res.status(404).send('Not implemented!!');
  });
  app.use(errorHandler);
  return app;
}

module.exports = { createBaseApp, finalizeApp };
