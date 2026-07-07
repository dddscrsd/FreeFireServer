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

// join routes a joining human to a match — an existing one with a free slot, else a fresh one — and
// admits them. m.reserved (manager-owned under mu) is the race-safe roster-size view; run() owns
// m.players. The first player of a fresh match runs admitFirst inline (which starts run()); a later
// player's admit is enqueued onto the already-running match loop, and its inbound handlers are routed
// to that mailbox from now on (syncStarted).
func (mgr *MatchManager) join(s *session) {
	mgr.mu.Lock()
	var m *Match
	for _, cand := range mgr.matches {
		if !cand.ended && cand.reserved < maxPlayers {
			m = cand
			break
		}
	}
	fresh := m == nil
	if fresh {
		m = newMatch(s)
		mgr.matches = append(mgr.matches, m)
	}
	m.reserved++
	mgr.mu.Unlock()

	s.match = m
	if fresh {
		m.admitFirst(s) // sets up the world + starts run()
		return
	}
	s.syncStarted = true // route THIS player's inbound handlers to the shared run() mailbox
	s.enqueue(func() { m.admitLater(s) })
}

// register adds a match to the registry and reserves a slot — used by the reconnect resume, which
// creates its own solo match outside join().
func (mgr *MatchManager) register(m *Match) {
	mgr.mu.Lock()
	mgr.matches = append(mgr.matches, m)
	m.reserved++
	mgr.mu.Unlock()
}

// reap removes a finished match from the registry — called from its run() goroutine on exit. It's a
// brief, once-per-match lock, so it doesn't contend with the hot single-owner path inside a match.
func (mgr *MatchManager) reap(m *Match) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	kept := mgr.matches[:0]
	for _, cand := range mgr.matches {
		if cand != m {
			kept = append(kept, cand)
		}
	}
	mgr.matches = kept
}

// count returns the number of live matches (diagnostics / future reaping).
func (mgr *MatchManager) count() int {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	return len(mgr.matches)
}
