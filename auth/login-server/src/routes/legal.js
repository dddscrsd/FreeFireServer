'use strict';

const renderTerms = require('../views/legal/terms');
const renderPrivacy = require('../views/legal/privacy');

module.exports = function registerLegalRoutes(app) {
  app.get('/terms', (req, res) => {
    res.status(200).type('html').send(renderTerms());
  });
  app.get('/privacy', (req, res) => {
    res.status(200).type('html').send(renderPrivacy());
  });
};
