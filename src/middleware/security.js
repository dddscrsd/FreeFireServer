const helmet = require('helmet');
const cors = require('cors');
const rateLimit = require('express-rate-limit');

module.exports = function applySecurity(app, opts = {}) {
  app.use(helmet());
  app.use(cors(opts.cors || {}));

  const limiter = rateLimit(
    Object.assign(
      {
        windowMs: 15 * 60 * 1000,
        max: 100
      },
      opts.rateLimit || {}
    )
  );

  app.use(limiter);
};
