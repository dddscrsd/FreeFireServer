require('dotenv').config();
const config = require('../../config/default');
const { startServer } = require('./_start');

startServer(require('../apps/liveApp')(), config.ports.live, 'live');
