require('dotenv').config();
const config = require('../../config/default');
const { startServer } = require('./_start');

startServer(require('../apps/loginApp')(), config.ports.login, 'login');
