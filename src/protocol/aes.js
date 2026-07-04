const crypto = require('crypto');

const config = require('../../config/default');

// Confirmed in HttpManager::Init (see protocol/SPEC.md):
//   AES-128-CBC, PKCS7 padding, static key + IV for every request.
const ALGORITHM = 'aes-128-cbc';
const KEY = Buffer.from('H*JiOpjzB^6JfLnJ', 'ascii'); // 482a4a694f706a7a425e364a664c6e4a
const IV = Buffer.from('knVpV!&My7#q0MiH', 'ascii'); // 6b6e56705621264d79372371304d6948

// 'raw' (default) -> body is raw ciphertext bytes; 'base64' -> body is base64 text.
const BODY_ENCODING = (config.protocol && config.protocol.bodyEncoding) || 'raw';

/**
 * AES-128-CBC encrypt a plaintext buffer. PKCS7 padding (node default).
 * @param {Buffer} buf plaintext
 * @returns {Buffer} ciphertext
 */
function encrypt(buf) {
  const cipher = crypto.createCipheriv(ALGORITHM, KEY, IV);
  return Buffer.concat([cipher.update(buf), cipher.final()]);
}

/**
 * AES-128-CBC decrypt a ciphertext buffer. PKCS7 padding (node default).
 * @param {Buffer} buf ciphertext
 * @returns {Buffer} plaintext
 */
function decrypt(buf) {
  const decipher = crypto.createDecipheriv(ALGORITHM, KEY, IV);
  return Buffer.concat([decipher.update(buf), decipher.final()]);
}

module.exports = {
  encrypt,
  decrypt,
  BODY_ENCODING,
  ALGORITHM,
  KEY,
  IV
};
