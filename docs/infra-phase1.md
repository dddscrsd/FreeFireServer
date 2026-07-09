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

## Run it (Postgres up, e.g. from the compose stack)
```sh
export DATABASE_URL=postgres://ff:PASS@localhost:5432/freefire
npm run migrate      # apply migrations/*.sql (tracked in _migrations)
npm run repo-smoke   # exercise every repo method against real Postgres, then clean up
```

## Next
Step 3 wires the repository behind `src/db/player.js` — the async ripple through
the ~130 call sites via the `savePlayer` hook + `getByToken`. Step 4 migrates
`accounts.db` → Postgres; step 5 is the parity harness before cutover.
