package main

// Process seam (MULTIPLAYER_PLAN.md Step 8). The MatchManager routes and reaps matches through the
// MatchHandle interface and mints them through a MatchFactory, so it never names the concrete match
// type. The only implementation today is the in-process *Match (methods below), built by localFactory.
// A future procMatch could run a match in its own OS process on its own UDP port behind the SAME
// interface, and the manager's join/reap logic would not change — that is the point of the seam.
//
// SCOPE: this abstracts the manager's ADMISSION + LIFECYCLE surface only. The gameplay hot path (a
// session's inbound packets -> its *Match mailbox via s.match, which is typed *Match) is still
// concrete, so actually placing a match out-of-process would additionally need a transport that
// forwards those packets to the match's process. That is deliberately left as future work; Step 8
// only fixes the manager boundary so the rest can follow without touching routing.

// MatchHandle is the manager's view of a match: can it take another player, claim a slot, and wire a
// player in. Reaping compares handles by identity, so it needs no method here.
type MatchHandle interface {
	canAdmit() bool               // not ended and below the roster cap
	reserve()                     // claim a roster slot (called under the manager lock)
	admit(s *session, fresh bool) // wire the player in; fresh == its first player (starts run())
}

// MatchFactory mints a fresh handle for a match's first player. Swapping the factory (local -> proc)
// is the single change needed to place new matches out-of-process; nothing in the manager moves.
type MatchFactory interface {
	create(first *session) MatchHandle
}

// localFactory makes in-process matches — the *Match implements MatchHandle directly.
type localFactory struct{}

func (localFactory) create(first *session) MatchHandle { return newMatch(first) }

// --- in-process MatchHandle: the *Match itself is the handle -----------------

func (m *Match) canAdmit() bool { return !m.ended && m.reserved < maxPlayers }

func (m *Match) reserve() { m.reserved++ }

// admit wires a joining player into this in-process match. The first player runs admitFirst inline
// (which starts run()); a later player's admit is enqueued onto the already-running loop and its
// inbound handlers route to that mailbox from now on (syncStarted).
func (m *Match) admit(s *session, fresh bool) {
	s.match = m
	if fresh {
		m.admitFirst(s)
		return
	}
	s.syncStarted = true
	s.enqueue(func() { m.admitLater(s) })
}
