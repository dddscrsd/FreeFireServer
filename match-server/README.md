# Contra Squad Match Server

A Go UDP match server that reimplements the Free Fire 1.70.1 **Contra Squad**
(game mode 15) match protocol, making the retail client fully playable against a
server-driven bot. Listens on UDP **:10100**.

## Architecture

| Path | Responsibility |
|------|----------------|
| `main.go` | UDP listener + session dispatch |
| `session.go` | per-client session state |
| `handlers.go` | inbound packet dispatch (join, damage, position, quit, …) |
| `packet/` | RUDP framing — msg key, CRC7, TEA, the cmd enum |
| `message/` | typed message builders (join, PRI/GRI, death, revive, shop, safezone, …) |
| `crypto/` | CRC7 + TEA |
| `token/` | cmd 440 prepare_token (JWT) |
| `player.go`, `spawns.go` | player + bot entities, arenas / gates |
| `cssync.go` | CS state stream + round loop (Prepare → Fight → transition) |
| `round.go` | between-rounds flow (banner, fade, revive/teleport, match end) |
| `combat.go` | HP, damage (cmd 106), death (cmd 107) |
| `safezone.go` | shrinking safe zone (cmd 117) + out-of-zone damage |
| `shop.go`, `purchase.go` | buy-phase shop (cmd 407/408) + inventory |
| `config.go` | environment config |

## Running

```sh
go build -o match-server.exe . && ./match-server.exe   # or: go run .
```

Config is via environment (all optional; see `config.go`):

| Env | Effect |
|-----|--------|
| `MATCH_JWT_SECRET` | secret that verifies the cmd 440 prepare_token |
| `MATCH_SPAWN="x,y,z"` | force a spawn instead of a random arena (`MATCH_SPAWN_FACE` sets facing) |
| `MATCH_HOLD_PREPARE=1` | stay in the Prepare/shop phase (shop testing) |
| `MATCH_ZONE_TEST=1` | offset the zone so a stationary player dies to it fast (death/revive testing) |
| `MATCH_DEBUG_POS=1` | log every parsed client position (cmd 1001) |

## What works

Join + spawn, PRI/GRI replication, movement, HP/damage, kills + kill feed
(including headshots), the enemy bot, the shrinking safe zone, the buy-phase shop
with purchases + per-round coins, the full round loop (VICTORY/DEFEAT banner, fade
transition, **local-player revive**), the CS match timer, and match end.

## Notable protocol notes

- **Round respawn** = a pending-revive death (cmd 107, pending flag set) followed by
  **cmd 388 NOTIFYREVIVE** at round start. The revive reuses the existing player
  object, so it does *not* re-team — the enemy bot stays hostile (unlike a cmd 101
  re-join, which glitches the bot onto the player's team).
- **Match end** = cmd 103 (per-client rank: 1 = win, 2 = lose).
- **Safe zone** = cmd 117; the server applies the out-of-zone damage (the client only
  renders + shrinks the circle).
- **CS clock** = cmd 1000 SyncServerTime (30 Hz); every countdown = deadline − clock.
- **Hit body part** (headshots) is byte 9 of the cmd 106 damage report.

The RE'd wire layouts and the il2cpp addresses they came from are documented inline
in the code comments.
