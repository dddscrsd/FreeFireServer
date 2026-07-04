require('dotenv').config();
const config = require('../../config/default');
const { startServer } = require('./_start');

startServer(require('../apps/mainApp')(), config.ports.main, 'main');
