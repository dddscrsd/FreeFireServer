require('dotenv').config();
const path = require('path');
const { fork } = require('child_process');
const logger = require('./logger');

// Launcher: run the three split servers as child processes. `npm start` boots
// all of them; each can also be run on its own via `npm run live|login|main`.
//   live  (3000): /live/ver.php
//   login (3001): MajorLogin / MajorRegister / Login / PlatformLogin / PlatformRegister
//   main  (3002): every other game endpoint + /api
const SERVERS = [
  { name: 'live', file: 'servers/live.js' },
  { name: 'login', file: 'servers/login.js' },
  { name: 'main', file: 'servers/main.js' },
  { name: 'tcp', file: 'servers/tcp.js' }
];

const children = [];
let shuttingDown = false;

function shutdownAll(exitCode) {
  if (shuttingDown) return;
  shuttingDown = true;
  for (const c of children) {
    try { c.kill('SIGTERM'); } catch (e) { /* already gone */ }
  }
  setTimeout(() => process.exit(exitCode || 0), 1500).unref();
}

for (const s of SERVERS) {
  const child = fork(path.join(__dirname, s.file), [], { stdio: 'inherit' });
  children.push(child);
  child.on('exit', (code, signal) => {
    logger.error(`[launcher] ${s.name} exited (code=${code}, signal=${signal || 'none'})`);
    // If a child dies unexpectedly, tear the whole group down so the operator
    // notices instead of running a partial stack.
    if (!shuttingDown) shutdownAll(1);
  });
}

process.on('SIGINT', () => { logger.info('[launcher] SIGINT received'); shutdownAll(0); });
process.on('SIGTERM', () => { logger.info('[launcher] SIGTERM received'); shutdownAll(0); });

logger.info(`[launcher] started ${SERVERS.length} servers: ${SERVERS.map((s) => s.name).join(', ')}`);
