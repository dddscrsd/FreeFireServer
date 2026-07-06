package main

import "sync"

// maxPlayers caps a CS match's roster at the user's 8-player limit; the minimum of 2 comes for
// free because a lone human always has the enemy bot for company. The MatchManager enforces the
// cap when routing a joining player (Step 4b wires the routing; until Step 5 there is only ever
// one human per match, so the cap is not yet exercised).
const maxPlayers = 8

// MatchManager owns every live Match in the process and routes a joining player to one: an
// existing match with a free slot, else a fresh match. Step 4b introduces it as the central
// registry (the match list was implicit in main.go's sessions map before); Step 5 uses join()
// to drop a second player into an existing match; Step 7 reaps finished matches from it.
//
// mu guards the match list. It is touched from the receive goroutine at join/registration time,
// NEVER from a match's run() loop (a match never reaches across to another), so it never
// contends with the lock-free single-owner model inside a match.
type MatchManager struct {
	mu      sync.Mutex
	matches []*Match
}

// matchManager is the process-wide registry.
var matchManager = &MatchManager{}

// register adds a newly created match to the registry.
func (mgr *MatchManager) register(m *Match) {
	mgr.mu.Lock()
	mgr.matches = append(mgr.matches, m)
	mgr.mu.Unlock()
}

// count returns the number of live matches (diagnostics / future reaping).
func (mgr *MatchManager) count() int {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	return len(mgr.matches)
}
