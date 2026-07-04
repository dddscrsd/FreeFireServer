const http = require('http');
const logger = require('../logger');

// Start an HTTP server for `app` on `port`. Keep-alive timeouts are raised well
// above Node's 5s default (headersTimeout must be >= keepAliveTimeout) so any
// residual keep-alive traffic can't hit the close-vs-reuse race that surfaces as
// intermittent "stream closed" on the game client. (Game responses already send
// Connection: close; see src/protocol/router.js.)
function startServer(app, port, name) {
  const server = http.createServer(app);
  server.keepAliveTimeout = 65000;
  server.headersTimeout = 66000;

  server.listen(port, () => {
    logger.info(`[${name}] listening on port ${port}`);
  });

  function shutdown(signal) {
    logger.info(`[${name}] received ${signal}. Shutting down gracefully.`);
    server.close((err) => {
      if (err) {
        logger.error(`[${name}] error during shutdown`, err);
        process.exit(1);
      }
      logger.info(`[${name}] shutdown complete.`);
      process.exit(0);
    });
  }
  process.on('SIGINT', () => shutdown('SIGINT'));
  process.on('SIGTERM', () => shutdown('SIGTERM'));

  return server;
}

module.exports = { startServer };
