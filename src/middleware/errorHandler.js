const logger = require('../logger');

module.exports = function errorHandler(err, req, res, next) {
  const status = err.status || 500;
  const payload = {
    message: err.message || 'Internal Server Error'
  };

  if (process.env.NODE_ENV !== 'production') payload.stack = err.stack;

  logger.error('Request error', { err });
  res.status(status).json(payload);
};
