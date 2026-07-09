'use strict';

const {
  config,
  logger,
  mongo,
  createGuestsRepo,
  createAccountsRepo,
  createPairingsRepo,
} = require('@auth/shared');
const createApp = require('./app');

const SERVICE = 'login-server';

async function main() {
  const { db } = await mongo.connect();
  await mongo.ensureIndexes(db);

  const guests = createGuestsRepo(db);
  const accounts = createAccountsRepo(db);
  const pairings = createPairingsRepo(db);
  const app = createApp({ guests, accounts, pairings });

  const server = app.listen(config.LOGIN_PORT, () => {
    logger.info(
      { service: SERVICE, port: config.LOGIN_PORT, callback: config.DISCORD_CALLBACK_PATH },
      'listening'
    );
  });

  let shuttingDown = false;
  async function shutdown(signal) {
    if (shuttingDown) return;
    shuttingDown = true;
    logger.info({ service: SERVICE, signal }, 'shutdown initiated');
    server.close(() => {});
    setTimeout(async () => {
      try { await mongo.close(); } catch (err) { logger.error({ err }, 'mongo close failed'); }
      process.exit(0);
    }, 1000).unref();
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
