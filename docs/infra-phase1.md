# Infra migration — Phase 1 (Data layer)

A Postgres schema + an async repository that mirrors the SQLite store, so the two
run side by side until the cutover. **Steps 1–2 (schema + repository) landed**;
steps 3–5 follow. Nothing changes for the live app yet — with `DATABASE_URL`
unset the repository uses the existing SQLite backend.

## Schema (`migrations/`)
- `001_accounts.sql` — `accounts`: identity columns + the whole player document
  as JSONB `state` (1:1 with the SQLite blob), a `token` index, and a `pg_trgm`
  nickname index (replaces the `LIKE COLLATE NOCASE` scan).
- `002_promoted.sql` — `wallets`, `friendships`, `friend_requests`, `clans`,
  `clan_members`, `match_results` (the idempotency ledger). Scaffolding for now;
  the friend/clan/wallet handlers move onto them incrementally.

## Repository (`src/db/repo`)
One async interface, two backends selected by `DATABASE_URL`:
- `postgres.js` — the `pg` implementation; mirrors `player.js` exactly.
- `sqlite.js` — async facade over the current `better-sqlite3` store (the default).
- `index.js` — `getRepo()` picks the backend.

Interface: `getByToken`, `getById`, `getByOpenId`, `getByIds` (batch — kills the
N+1 friend/clan fan-outs), `searchByNickname`, `createFromLogin`, `save`, `close`.

## Cutover procedure (Postgres up, e.g. from the compose stack)
```sh
export DATABASE_URL=postgres://ff:PASS@localhost:5432/freefire
npm run migrate        # 1. apply migrations/*.sql (schema; tracked in _migrations)
npm run migrate-data   # 2. copy accounts.db -> Postgres (idempotent; resets the id sequence)
npm run repo-smoke     # (optional) exercise the repo against real Postgres, then clean up
# 3. restart the app with DATABASE_URL set -> the HTTP tier now runs on Postgres.
```
With `DATABASE_URL` unset the app stays on SQLite unchanged.

## Status
- **Step 1–2 (schema + repository)** — done.
- **Step 3 (HTTP tier on the async repo)** — done. The router resolves auth via
  the repo and persists via a deferred save; the direct-caller handlers `await
  getRepo().*`.
- **Step 4 (data migration)** — done (`src/db/migrate-data.js`).
- **Remaining:** convert the **TCP gateway** (`src/tcp/gateway.js` `getByToken`)
  off the sync SQLite path before the cutover (else HTTP→PG but gateway→SQLite),
  then a parity check.
