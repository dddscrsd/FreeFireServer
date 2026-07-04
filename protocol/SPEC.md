# Free Fire 1.70.0 — server protocol spec (reverse-engineered)

Everything here is confirmed from the decrypted `libil2cpp.so` (IDA) and the
proto dump. Build the protobuf+AES layer on top of the existing Express base.

## Transport
- The client POSTs to `<ServerAddr>/<EndpointName>` — **the URL path is the endpoint
  name** (e.g. `POST /MajorLogin`). The existing `src/routes/login.js` already shows
  this (`/MajorRegister`).
- Request body = **AES( protobuf-serialize(requestMessage) )**.
- Response body = **AES( protobuf-serialize(responseMessage) )**.
- `Empty` responses (`proto.Empty`) mean "ack" (success/failure conveyed out-of-band).

## AES (confirmed in HttpManager::Init)
- Algorithm **AES-128-CBC**, padding **PKCS7**.
- Key (16 bytes, ASCII): `H*JiOpjzB^6JfLnJ`  (hex `482a4a694f706a7a425e364a664c6e4a`)
- IV  (16 bytes, ASCII): `knVpV!&My7#q0MiH`  (hex `6b6e56705621264d79372371304d6948`)
- Static key/IV (same for every request). Body is the raw AES ciphertext bytes
  (Content-Type application/octet-stream). If a captured request turns out to be
  base64-wrapped, make the codec try base64 too — keep it configurable.

## Protobuf
- Definitions: `protocol/protos/{proto,tcp,message,COW}.proto` (proto3, compile-clean).
  Packages: `proto` (main, 1828 msgs), `tcp`, `message`, `COW`. Load all four with
  protobufjs (`protobufjs`'s `load()` resolves the imports). Most req/res types live
  in package `proto`.

## Endpoint map
- `protocol/endpoint_map.json`: array of `{cmd, cmd_hex, caller, reqType, resType, endpoint}`.
  - `endpoint` = URL path to register (e.g. `MajorLogin`).
  - `reqType` = protobuf message to DECODE the request body into (package `proto` unless
    obvious otherwise; `null` ⇒ no/empty request body).
  - `resType` = protobuf message to ENCODE the response (e.g. `MajorLoginRes`, `Empty`).
  - 418 unique endpoints. Some `reqType`/`resType` may be null — default to `Empty`.

## Accounts database (organized)
- Persist accounts in a real DB (better-sqlite3 preferred; a JSON store is acceptable
  fallback). Schema driven by `proto.LoginReq` / `proto.MajorLoginRes` fields, e.g.:
  `account_id (PK), open_id, open_id_type, nickname, plat_id, region, language,
  device_id, client_version, level, clan_id, created_at, last_login_at, raw_login JSON`.
- On `MajorLogin`/`MajorRegister`: upsert the account by `open_id` (create with a fresh
  `account_id` if new), update `last_login_at`, return a populated `MajorLoginRes`
  (account_id etc.).

## Login-flow endpoints to implement for real (others may be typed stubs)
`MajorLogin`, `MajorRegister`, `Login`, `ChooseRegion`, `GetPlatformProfile`,
`GetLoginData`, `AccountMatchStats` — create/lookup the account and return a sensible
populated response; everything else: decode the request, return a default-constructed
`resType` (valid empty protobuf) so the client always gets a well-formed reply.

## Existing base (extend, don't rewrite)
- Express, CommonJS, `src/` (server.js, app.js, routes/, middleware/, services/, logger.js).
- Add: `src/protocol/` (proto loader + AES codec + router), `src/db/` (accounts),
  `src/handlers/` (per-group handler modules + a registry). Wire the protobuf router
  into `app.js` BEFORE the JSON 404. Keep the existing JSON routes working.
