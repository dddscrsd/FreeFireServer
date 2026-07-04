const path = require('path');
const protobuf = require('protobufjs');

const logger = require('../logger');

const PROTO_DIR = path.join(__dirname, '..', '..', 'protocol', 'protos');

// Package search order used by lookup(). Most request/response types live in
// package `proto`; the rest fall back to tcp / message / COW.
const PACKAGES = ['proto', 'tcp', 'message', 'COW'];

// Load all four definitions into a single Root. proto.proto imports tcp.proto,
// which protobufjs resolves relative to the importing file; message.proto and
// COW.proto are standalone. keepCase keeps the snake_case field names as defined
// in the .proto files (the DB layer and handlers rely on those names).
const root = new protobuf.Root();
root.loadSync(
  [
    path.join(PROTO_DIR, 'proto.proto'),
    path.join(PROTO_DIR, 'tcp.proto'),
    path.join(PROTO_DIR, 'message.proto'),
    path.join(PROTO_DIR, 'COW.proto')
  ],
  { keepCase: true }
);

logger.info('[protos] loaded proto/tcp/message/COW definitions');

/**
 * Resolve a protobuf message Type by (possibly unqualified) name.
 *
 * Tries, in order: the name as-is when it already contains a '.', then
 * "proto.<name>", "tcp.<name>", "message.<name>", "COW.<name>", and finally the
 * bare name. Returns the protobuf.Type, or null when nothing matches / the name
 * is falsy.
 *
 * @param {string|null|undefined} typeName
 * @returns {protobuf.Type|null}
 */
function lookup(typeName) {
  if (!typeName) return null;

  const candidates = [];
  if (typeName.indexOf('.') !== -1) candidates.push(typeName);
  for (const pkg of PACKAGES) candidates.push(`${pkg}.${typeName}`);
  candidates.push(typeName);

  for (const name of candidates) {
    let resolved = null;
    try {
      resolved = root.lookup(name);
    } catch (err) {
      resolved = null;
    }
    if (resolved && resolved instanceof protobuf.Type) return resolved;
  }
  return null;
}

/**
 * Resolve a protobuf Enum by (possibly unqualified) name, mirroring lookup().
 * Nested enums are addressed by their full path, e.g. "tcp.EProtocol.Proto".
 *
 * @param {string|null|undefined} typeName
 * @returns {protobuf.Enum|null}
 */
function lookupEnum(typeName) {
  if (!typeName) return null;

  const candidates = [];
  if (typeName.indexOf('.') !== -1) candidates.push(typeName);
  for (const pkg of PACKAGES) candidates.push(`${pkg}.${typeName}`);
  candidates.push(typeName);

  for (const name of candidates) {
    let resolved = null;
    try {
      resolved = root.lookup(name);
    } catch (err) {
      resolved = null;
    }
    if (resolved && resolved instanceof protobuf.Enum) return resolved;
  }
  return null;
}

module.exports = { root, lookup, lookupEnum, protobuf, PACKAGES };
