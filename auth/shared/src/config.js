'use strict';

// Load .env from process.cwd() before we touch process.env.
// Both services are launched from the reimplementation/ workspace root, so
// this resolves to reimplementation/.env when a local .env exists.
require('dotenv').config();

const REQUIRED = [
  'MONGO_URI',
  'DB_NAME',
  'APP_ID',
  'APP_SECRET',
  'DISCORD_CLIENT_ID',
  'DISCORD_CLIENT_SECRET',
  'DISCORD_REDIRECT_URI',
  'OAUTH_STATE_SECRET',
];

function fatal(msg) {
  // We deliberately write to stderr directly here because the logger has not
  // been constructed yet at the point config validation runs.
  process.stderr.write(`[config] fatal: ${msg}\n`);
  process.exit(1);
}

function readNumber(name, fallback) {
  const raw = process.env[name];
  if (raw === undefined || raw === '') return fallback;
  const n = Number(raw);
  if (!Number.isFinite(n) || !Number.isInteger(n)) {
    fatal(`${name} must be an integer, got: ${raw}`);
  }
  return n;
}

function readBool(name, fallback) {
  const raw = process.env[name];
  if (raw === undefined || raw === '') return fallback;
  return raw.toLowerCase() === 'true' || raw === '1';
}

for (const name of REQUIRED) {
  if (!process.env[name] || process.env[name].trim() === '') {
    fatal(`required environment variable is missing: ${name}`);
  }
}

// Validate the Discord redirect URI is a well-formed absolute URL so services
// can extract its path when mounting the callback route.
let discordCallbackPath;
try {
  discordCallbackPath = new URL(process.env.DISCORD_REDIRECT_URI).pathname;
  if (!discordCallbackPath || discordCallbackPath === '/') {
    fatal(`DISCORD_REDIRECT_URI must contain a path component, got: ${process.env.DISCORD_REDIRECT_URI}`);
  }
} catch (err) {
  fatal(`DISCORD_REDIRECT_URI is not a valid URL: ${err.message}`);
}

const config = Object.freeze({
  NODE_ENV: process.env.NODE_ENV || 'development',
  LOG_LEVEL: process.env.LOG_LEVEL || 'info',
  TRUST_PROXY: readBool('TRUST_PROXY', false),

  CONNECT_PORT: readNumber('CONNECT_PORT', 5000),
  LOGIN_PORT: readNumber('LOGIN_PORT', 25565),

  MONGO_URI: process.env.MONGO_URI,
  DB_NAME: process.env.DB_NAME,

  APP_ID: process.env.APP_ID,
  APP_SECRET: process.env.APP_SECRET,

  DISCORD_CLIENT_ID: process.env.DISCORD_CLIENT_ID,
  DISCORD_CLIENT_SECRET: process.env.DISCORD_CLIENT_SECRET,
  DISCORD_REDIRECT_URI: process.env.DISCORD_REDIRECT_URI,
  DISCORD_CALLBACK_PATH: discordCallbackPath,

  OAUTH_STATE_SECRET: process.env.OAUTH_STATE_SECRET,

  isProduction: (process.env.NODE_ENV || 'development') === 'production',
});

module.exports = config;
