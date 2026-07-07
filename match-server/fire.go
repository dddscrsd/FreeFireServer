package main

import (
	"encoding/binary"

	"libmadoka/match-server/packet"
)

// Weapon-fire + reload visual sync (CS-2). A remote pawn's muzzle flash / tracer / fire animation is
// produced by PlayerNetwork::SyncStartFire, which the shooter's edge events (cmd 104 start-fire /
// cmd 105 stop-fire) invoke on the OTHER clients — and SyncStartFire reads the pawn's replicated PRI
// field-21 START_FIRE_STATE. So the server both RELAYS the events to the others AND streams field 21
// non-zero while the trigger is held (see priPayload + session.firing). Reload = cmd 133/134, relayed
// the same way. The shooter is never echoed (it fires from its own local input). See cs-fire-sync.

// broadcastToOthers relays a client's packet verbatim to every OTHER roster client. Reliable +
// encrypted, matching how the shooter sent it.
func (m *Match) broadcastToOthers(sender *session, cmd uint16, payload []byte) {
	for _, p := range m.players {
		if p != sender && p.out != nil {
			p.out.send(&packet.Packet{SendOption: packet.SendReliable, Cmd: cmd, Flags: packet.FlagEncrypted, Payload: payload}, "")
		}
	}
}

// handleWeaponAction handles cmd 104 (multiplexed weapon action): relay it to the others so their
// SyncStartFire renders the muzzle/tracer, and if it's a START-FIRE (actionType 2) mark this player
// firing + push an eager PRI so field 21 is non-zero by the time the event lands.
func (s *session) handleWeaponAction(p *packet.Packet) {
	if len(p.Payload) < 16 {
		return
	}
	start := binary.LittleEndian.Uint32(p.Payload[12:]) == 2 // actionType 2 = START FIRE
	reps := 1
	if start {
		// Prime the pawn FIRST: field 21 (fire-state) + field 4 (equipped weapon) must be set before
		// the fire event lands, or SyncStartFire reads a 0 state — a gun then plays no shot/sound (a
		// fist fires as a one-shot regardless). The reference sends the weapon+state before cmd 104.
		s.firing = true
		s.sendVar(packet.CmdPRISync, s.match.priPayload(), 1)
		reps = 3 // then repeat the start-fire ~3x so the shot AUDIO registers (per the reference)
	}
	for i := 0; i < reps; i++ {
		s.match.broadcastToOthers(s, packet.CmdWeaponAction, p.Payload)
	}
}

// handleStopFire handles cmd 105: relay it + clear the firing flag (eager PRI so field 21 -> 0).
func (s *session) handleStopFire(p *packet.Packet) {
	s.match.broadcastToOthers(s, packet.CmdStopFire, p.Payload)
	s.firing = false
	s.sendVar(packet.CmdPRISync, s.match.priPayload(), 1)
}

// handleReloadRelay relays a reload event (cmd 133 start / 134 finish) to the other clients so they
// play the remote reload animation. No server state to track.
func (s *session) handleReloadRelay(p *packet.Packet) {
	s.match.broadcastToOthers(s, p.Cmd, p.Payload)
}
