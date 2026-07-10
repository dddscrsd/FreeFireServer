package main

import (
	"fmt"
	"log"
	"time"

	"libmadoka/match-server/bus/pb"
)

// Persistent match rewards (settlement) — DISTINCT from the in-match CS economy
// (startingCoins / p.coins, which buys weapons and never leaves the match). These
// credit the player's LOBBY account and are applied by the settlement worker
// (src/servers/settlement.js). Kept as simple constants for now; the worker records
// the full per-player result so the formula can move server-side later.
const (
	settleCoinPerKill = 50
	settleWinCoins    = 200
	settleXPPerKill   = 10
	settleWinXP       = 100
	settleLoseXP      = 40
)

// publishResult emits the durable match.result (per-player settlement) and match.ended
// events at match end. No-op when the bus is disabled (standalone). Fire-and-forget:
// Publish never blocks the match loop (see bus.Publish). The settlement worker is
// idempotent on (match_id, account_id), so an at-least-once redelivery can't double-credit.
func (m *Match) publishResult() {
	if eventBus == nil {
		return
	}
	matchID := m.matchIDString()
	players := make([]*pb.PlayerResult, 0, len(m.players))
	for _, p := range m.players {
		win := p.team == m.winnerTeam
		kills := int32(m.kills[p.entityID])
		coins := kills * settleCoinPerKill
		xp := kills*settleXPPerKill + settleLoseXP
		if win {
			coins += settleWinCoins
			xp = kills*settleXPPerKill + settleWinXP
		}
		players = append(players, &pb.PlayerResult{
			AccountId: int64(p.player.AccountID),
			Coins:     coins,
			Kills:     kills,
			Deaths:    int32(m.deaths[p.entityID]),
			Win:       win,
			Xp:        xp,
			Damage:    int32(m.damage[p.entityID]),
		})
	}
	eventBus.Publish("match.result", &pb.MatchResult{MatchId: matchID, Players: players})
	eventBus.Publish("match.ended", &pb.MatchEnded{
		MatchId:    matchID,
		WinnerTeam: uint32(m.winnerTeam),
		DurationMs: time.Since(m.matchStart).Milliseconds(),
		EndType:    4, // normal CS match end (mirrors the cmd 103 endType)
	})
	log.Printf("[mm-udp] bus: published match.result match=%s players=%d winner=%d", matchID, len(players), m.winnerTeam)
}

// matchIDString is the shared match id used as the settlement idempotency key: the
// prepare_token `mid`, which the matchmaker gives every player it puts in this match.
// A tokenless manual test has mid 0 → synthesise a per-instance id so two local test
// matches don't collide on the ledger PK.
func (m *Match) matchIDString() string {
	for _, p := range m.players {
		if p.player.MatchID != 0 {
			return fmt.Sprintf("%d", p.player.MatchID)
		}
	}
	return fmt.Sprintf("%s-%d", cfg.nodeID, m.matchStart.UnixNano())
}
