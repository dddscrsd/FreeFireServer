## Free Fire Server - HTTP skeleton

Minimal production-like Node.js HTTP server for a Free Fire (1.70.0) client,
split into three independent processes.

## Quick start

1. Install dependencies

```
npm install
```

2. Run all three servers (development, requires `nodemon`):

```
npm run dev
```

3. Run all three servers (production):

```
npm start
```

`npm start` / `npm run dev` launch a small supervisor (`src/server.js`) that
forks the three servers below and shuts them all down together.

## The three servers

| Server | Port | Owns |
|--------|------|------|
| `live`  | 3000 | `GET /live/ver.php` (version / bootstrap) |
| `login` | 3001 | `MajorLogin`, `MajorRegister`, `Login`, `PlatformLogin`, `PlatformRegister` |
| `main`  | 3002 | every other game endpoint (AES/protobuf) + the JSON `/api` routes |

Each also exposes `GET /health`. Ports are configurable via `LIVE_PORT` /
`LOGIN_PORT` / `MAIN_PORT` (see `.env.example`).

Run one server on its own:

```
npm run live     # port 3000
npm run login    # port 3001
npm run main     # port 3002
```

## Client redirect flow

The client is redirected from one server to the next via the `server_url` field
in each response:

```
client -> :3000  GET /live/ver.php     -> server_url = http://<ip>:3001/
client -> :3001  POST /MajorLogin ...   -> server_url = http://<host>:3002
client -> :3002  everything else
```

`server_url` derives the host from the incoming request and swaps in the target
port, so it works on localhost / LAN without hardcoding an IP. Set
`SERVER_URL` in the environment to force a fixed public URL (e.g. behind a proxy).

## Structure highlights

- `src/server.js` — launcher: forks the three servers + group shutdown
- `src/servers/` — per-process entry points (`live.js`, `login.js`, `main.js`) + shared `_start.js`
- `src/apps/` — Express app builders (`liveApp`, `loginApp`, `mainApp`, shared `base.js`)
- `src/protocol/router.js` — AES/protobuf `POST /:cmd` router factory (accepts a command filter)
- `src/protocol/authCommands.js` — the set of endpoints owned by the login server
- `src/handlers/` — one module per game endpoint
- `src/middleware` — logging, error handling
- `src/logger.js` — centralized logger (winston)
