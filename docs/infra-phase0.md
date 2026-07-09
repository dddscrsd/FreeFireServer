# Infra migration — Phase 0 (Foundations)

Part of the SQLite → PostgreSQL + Redis-bus migration (see the design doc). This
phase lays the plumbing; no runtime behaviour changes yet.

## What landed

- **Shared bus contract** — `proto/bus/envelope.proto` + `proto/bus/events.proto`.
  One source of truth for both languages: Go generates from it, Node loads it at
  runtime. Every message is a `bus.Envelope { type, source, correlation_id,
  ts_unix_ms, payload }`; the concrete event rides in `payload`.
- **Bus clients**
  - Go: `match-server/bus` (`bus.New`, `Publish`/`PublishPS`, `SubscribeStream`
    /`SubscribePS`, presence). Publishing is off the tick hot-path via a buffered
    drainer — a full buffer drops with a log, never blocks the match loop.
  - Node: `src/bus` (`new Bus()`, same surface). Binary-safe reads via ioredis
    Buffer variants.
  - Routing: durable commands → `stream:<type>` (XADD + consumer groups);
    ephemeral → `ps:<type>` (PUBLISH); presence → `presence:<accountID>` (TTL key).
- **Config** — `config/default.js` gains `redis`, `postgres`, `domains`, `nodeId`;
  the Go match-server gains `REDIS_URL` + `NODE_ID` (bus is optional — empty
  `REDIS_URL` keeps it standalone). New vars documented in `.env.example`.

## Regenerate the Go bindings

Node needs no codegen. For Go:

```sh
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
export PATH="$PATH:$(go env GOPATH)/bin"
make proto        # → match-server/bus/pb/{envelope,events}.pb.go
```

## Smoke test (needs a running Redis)

```sh
# terminal 1 — Go consumer
cd match-server && go run ./cmd/bussmoke consume
# terminal 2 — Node publisher (cross-language)
node scripts/bus-smoke.js publish
```

The Go side should print the Node `Ping`, proving the shared envelope contract
round-trips across both clients.

## Still in Phase 0 (edge/deploy, next commit)

`docker-compose` (Postgres, Redis, Traefik) + a `Dockerfile` per module, the
per-domain Traefik routing, and the GitHub Actions → GHCR → VPS deploy workflow.
