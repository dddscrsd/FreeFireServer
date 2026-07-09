// Loads the SHARED bus contract (proto/bus/*.proto) via protobufjs — the same
// .proto files that generate the Go side, so there is one source of truth. Field
// names stay snake_case (keepCase) to match the .proto and the rest of the Node
// tier, so callers build plain objects like { match_id, players: [...] }.
const path = require('path');
const protobuf = require('protobufjs');

const PROTO_DIR = path.join(__dirname, '..', '..', 'proto', 'bus');

const root = new protobuf.Root();
root.loadSync(
  [path.join(PROTO_DIR, 'envelope.proto'), path.join(PROTO_DIR, 'events.proto')],
  { keepCase: true }
);

const Envelope = root.lookupType('bus.Envelope');

// Resolve a payload message type by short ("Ping") or qualified ("bus.Ping") name.
function type(name) {
  return root.lookupType(name.indexOf('.') === -1 ? `bus.${name}` : name);
}

// Encode a payload object to bytes, verifying it against the schema first.
function encode(typeName, obj) {
  const T = type(typeName);
  const bad = T.verify(obj);
  if (bad) throw new Error(`bus encode ${typeName}: ${bad}`);
  return T.encode(T.create(obj)).finish();
}

// Decode payload bytes into a plain object.
function decode(typeName, buf) {
  const T = type(typeName);
  return T.toObject(T.decode(buf), { longs: Number, defaults: true });
}

module.exports = { root, Envelope, type, encode, decode };
