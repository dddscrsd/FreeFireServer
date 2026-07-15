package main

import (
	"log"

	"libmadoka/match-server/bus"
	"libmadoka/match-server/bus/pb"
)

// startAdminKick subscribes to the session.revoke bus channel so an admin action
// (or the login tier's ban/kick) can eject a player who is mid-match. The Node TCP
// gateway already handles the lobby socket for the same message; this handles the
// UDP match side. No-op when the bus is disabled (standalone mode).
func startAdminKick() {
	if eventBus == nil {
		return
	}
	eventBus.SubscribePS("session.revoke", func(env *pb.Envelope) {
		var m pb.SessionRevoke
		if err := bus.Decode(env, &m); err != nil {
			return
		}
		kickByAccount(m.GetAccountId(), m.GetReason())
	})
	log.Printf("[mm-udp] admin: subscribed to session.revoke (match kick)")
}

// kickByAccount ejects the UDP session for accountID, if one is connected to THIS
// instance. Sends SendKick (the client disconnects) and, when the player is already
// in a running match, drops them from the roster on the match goroutine (via the
// mailbox, like every other match-state mutation) so survivors see them leave.
func kickByAccount(accountID int64, reason string) {
	if accountID == 0 {
		return
	}
	sessionsMu.Lock()
	var target *session
	for _, s := range sessions {
		if int64(s.player.AccountID) == accountID {
			target = s
			break
		}
	}
	sessionsMu.Unlock()
	if target == nil {
		return // not on this instance
	}
	log.Printf("[mm-udp] admin kick uid=%d (%s)", accountID, reason)
	target.kick()
	if target.syncStarted && target.match != nil {
		target.enqueue(func() { target.match.removePlayer(target) })
	}
}
