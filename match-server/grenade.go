package main

import "libmadoka/match-server/packet"

// Grenade / throwable sync. A throwable is a replication-entity projectile (LevelProjectile): the
// thrower's client reports discrete events and every OTHER client spawns + simulates the grenade
// locally from them. The server just RELAYS each event to the others (the thrower simulates its own,
// and ignores its own echo). Damage is NOT here — it rides the normal cmd 106 path like bullets.
// See cs-grenade-sync. Relayed cmds: 160 THROW (spawns the arc), 161 EXPLODE (blast FX), 162/163
// START/STOP (pull-pin windup). cmd 159 STATE_SYNC (mid-air position) is skipped — remotes arc the
// grenade from the throw alone, and relaying a high-frequency stream reliably isn't worth it.

// handleProjectileRelay relays a grenade event (cmd 160/161/162/163) to the other roster clients so
// remotes see the thrower's grenade arc, explode, and wind up. The thrower is not echoed (its own
// client already simulates the grenade locally).
func (s *session) handleProjectileRelay(p *packet.Packet) {
	s.match.broadcastToOthers(s, p.Cmd, p.Payload)
}
