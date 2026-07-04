const { createBaseApp, finalizeApp } = require('./base');

// live server (port 3000): only the /live version endpoint (ver.php). Its
// server_url response redirects the client to the login server (port 3001).
module.exports = function createLiveApp() {
  const app = createBaseApp();
  app.use('/live', require('../routes/version'));
  return finalizeApp(app, 'live');
};
