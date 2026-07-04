require('dotenv').config();
const config = require('../../config/default');
const logger = require('../logger');
const { createGateway } = require('../tcp/gateway');

const server = createGateway();
server.listen(config.ports.tcp, () => {
  logger.info(`[tcp] gateway listening on port ${config.ports.tcp}`);
});

function shutdown(signal) {
  logger.info(`[tcp] received ${signal}. Shutting down gracefully.`);
  server.close(() => {
    logger.info('[tcp] shutdown complete.');
    process.exit(0);
  });
  setTimeout(() => process.exit(0), 1000).unref();
}
process.on('SIGINT', () => shutdown('SIGINT'));
process.on('SIGTERM', () => shutdown('SIGTERM'));
