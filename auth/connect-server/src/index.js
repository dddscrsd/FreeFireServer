'use strict';

const { config, logger, store, createGuestsRepo } = require('@auth/shared');
const createApp = require('./app');

const SERVICE = 'connect-server';

async function main() {
  const handle = await store.connect();
  await store.ensureSchema(handle);
  logger.info({ service: SERVICE, backend: handle.backend }, 'storage connected');

  const guests = createGuestsRepo(handle);
  const app = createApp({ guests });

  const server = app.listen(config.CONNECT_PORT, () => {
    logger.info({ service: SERVICE, port: config.CONNECT_PORT }, 'listening');
  });

  let shuttingDown = false;
  async function shutdown(signal) {
    if (shuttingDown) return;
    shuttingDown = true;
    logger.info({ service: SERVICE, signal }, 'shutdown initiated');
    server.close(() => {});
    // Give in-flight requests a moment, then close the storage backend.
    setTimeout(async () => {
      try { await store.close(); } catch (err) { logger.error({ err }, 'storage close failed'); }
      process.exit(0);
    }, 1000).unref();
    // Hard exit if shutdown takes too long.
    setTimeout(() => {
      logger.warn('forced exit after shutdown timeout');
      process.exit(1);
    }, 10_000).unref();
  }

  process.on('SIGINT', () => shutdown('SIGINT'));
  process.on('SIGTERM', () => shutdown('SIGTERM'));
  process.on('unhandledRejection', (err) => {
    logger.error({ err }, 'unhandledRejection');
  });
  process.on('uncaughtException', (err) => {
    logger.error({ err }, 'uncaughtException');
    shutdown('uncaughtException');
  });
}

main().catch((err) => {
  logger.error({ err, service: SERVICE }, 'startup failed');
  process.exit(1);
});
