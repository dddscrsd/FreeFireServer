'use strict';

// Endpoints owned by the LOGIN server (port 3001). Everything else is served by
// the MAIN server (port 3002). Login/PlatformLogin/PlatformRegister are aliases
// of MajorLogin/MajorRegister and are grouped here with them.
const AUTH_COMMANDS = new Set([
  'MajorLogin',
  'MajorRegister',
  'Login',
  'PlatformLogin',
  'PlatformRegister'
]);

module.exports = { AUTH_COMMANDS };
