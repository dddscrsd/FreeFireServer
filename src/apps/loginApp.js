const { createBaseApp, finalizeApp } = require('./base');
const createProtocolRouter = require('../protocol/router');
const { AUTH_COMMANDS } = require('../protocol/authCommands');

// login server (port 3001): the AES/protobuf router, restricted to the auth
// endpoints (MajorLogin/MajorRegister/Login/PlatformLogin/PlatformRegister).
// Mounted at '/' BEFORE any body parser so express.raw() captures the raw
// request stream (see protocol/router.js), and before helmet/cors so game
// responses stay clean. Anything not in AUTH_COMMANDS falls through to the 404.
module.exports = function createLoginApp() {
  const app = createBaseApp();
  app.use('/', createProtocolRouter({ filter: (cmd) => AUTH_COMMANDS.has(cmd) }));
  return finalizeApp(app, 'login');
};
