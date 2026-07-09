'use strict';

const crypto = require('crypto');

const OPEN_ID_ALPHABET = '0123456789abcdefghijklmnopqrstuvwxyz';
const OPEN_ID_LENGTH = 32;
const FIRST_UID = 10_000_001;

// Generate a 32-character lowercase alphanumeric identifier. crypto.randomInt
// is unbiased by construction (uses rejection sampling internally).
function generateOpenId() {
  let out = '';
  for (let i = 0; i < OPEN_ID_LENGTH; i++) {
    out += OPEN_ID_ALPHABET[crypto.randomInt(0, OPEN_ID_ALPHABET.length)];
  }
  return out;
}

// Given a guests repository, compute the next UID by looking at the maximum
// numeric value currently stored. UID is stored as a string in MongoDB so we
// convert during aggregation (see repositories/guests.js). Returns a string.
async function nextUid(guests) {
  const max = await guests.currentMaxUid();
  if (max === null || max === undefined) return String(FIRST_UID);
  return String(max + 1);
}

// Retry a fn if it fails with a MongoDB duplicate-key error, typically caused
// by two concurrent registrations racing to insert the same uid or dc_id.
async function withUidRetry(fn, { attempts = 5 } = {}) {
  let lastErr;
  for (let i = 0; i < attempts; i++) {
    try {
      return await fn();
    } catch (err) {
      if (err && err.code === 11000) {
        lastErr = err;
        continue;
      }
      throw err;
    }
  }
  throw lastErr || new Error('withUidRetry: attempts exhausted');
}

module.exports = { generateOpenId, nextUid, withUidRetry, FIRST_UID };
