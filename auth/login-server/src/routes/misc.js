'use strict';

module.exports = function registerMiscRoutes(app) {
  app.get('/health', (req, res) => {
    res.status(200).json({ status: 'ok', service: 'login-server' });
  });

  // Redirect the bare root to something informative rather than 404-ing.
  app.get('/', (req, res) => {
    res.status(200).type('html').send(
      '<!doctype html><html><head><meta charset="utf-8"><title>Auth server</title></head>' +
      '<body style="font-family:sans-serif;color:#999;background:#0d0d0d;padding:32px;">' +
      '<p>This is the authentication endpoint. It is reached by the game client, not by browsers directly.</p>' +
      '<p><a style="color:#ff6600;" href="/terms">Terms</a> &middot; ' +
      '<a style="color:#ff6600;" href="/privacy">Privacy</a></p>' +
      '</body></html>'
    );
  });
};
