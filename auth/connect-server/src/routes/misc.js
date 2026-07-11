'use strict';

// Literal-path routes registered ahead of any parameterized catch-alls so
// they win the match.
module.exports = function registerMiscRoutes(app) {
  app.get('/health', (req, res) => {
    res.status(200).json({ status: 'ok', service: 'connect-server' });
  });

  app.get('/app/info/get', (req, res) => {
    res.status(200).json({ status: 0, client_log: false });
  });
};
