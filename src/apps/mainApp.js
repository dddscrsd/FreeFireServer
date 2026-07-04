const { createBaseApp, finalizeApp } = require('./base');
const createProtocolRouter = require('../protocol/router');
const { AUTH_COMMANDS } = require('../protocol/authCommands');

// main server (port 3002): the AES/protobuf router for every NON-auth endpoint,
// plus the legacy JSON /api routes. The protocol router is mounted at '/' FIRST
// (before /api and before any body parser) so its express.raw() owns the raw
// game-protocol bodies; it calls next() for anything it doesn't match (auth
// commands, /api, /health).
module.exports = function createMainApp() {
  const app = createBaseApp();
  app.use('/', createProtocolRouter({ filter: (cmd) => !AUTH_COMMANDS.has(cmd) }));
  app.use('/api', require('../routes/index'));
  return finalizeApp(app, 'main');
};
