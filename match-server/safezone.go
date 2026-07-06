package main

import (
	"fmt"
	"math"
	"time"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// SafeZone tuning for Contra Squad. The zone is server-authoritative: the client only
// renders the circle + shows the timer; it never deducts HP itself, so we apply the
// out-of-zone damage here (CS is a fixed 50 HP every 3s).
const (
	zoneMargin      = 45.0             // metres of buffer beyond the gates for the outer circle
	zoneInnerRatio  = 0.01             // fully-shrunk radius as a fraction of the outer
	zoneDefaultR    = 70.0             // outer radius when there is no arena (fixed MATCH_SPAWN)
	zoneWaitDur     = 90 * time.Second // Fight time before the zone starts shrinking
	zoneShrinkDur   = 25 * time.Second // time to shrink outer -> inner
	zoneDamage      = 50               // HP lost per tick outside the zone (CS fixed)
	zoneDamageEvery = 2 * time.Second  // damage interval (CS fixed)

	// zoneClientLag compensates for the client rendering the shrinking circle a little
	// behind our real-time damage clock. The client lerps its radius over [StartTime,
	// EndTime] using CurrentServerTime (SafeZone::OnFixedUpdate) — a local sim clock
	// re-anchored only every cmd-1000 tick — so it trails our clock by the network latency
	// plus sim jitter, leaving its VISIBLE radius larger than a same-instant lerp. We
	// evaluate the damage radius this far in the past so it never shrinks past the circle
	// the player still sees; otherwise you take damage while visibly inside the zone. Tune
	// up if damage still lands inside the circle, down if you can stand safely outside it.
	zoneClientLag = 300 * time.Millisecond
)

// zoneGeometry returns the circle centre + outer radius for the current round: the
// arena's play-area centre sized to cover both gates, or a default circle around the
// fixed spawn when there is no arena (MATCH_SPAWN test).
func (s *session) zoneGeometry() (center message.Vec3, outerR float64) {
	if cfg.zoneTest { // debug: put the circle 100m away so the player spawns OUTSIDE it and dies fast
		c := s.player.SpawnPos
		c.X += 100
		return c, 40
	}
	if s.match.arena.City != "" {
		return s.match.arena.center(), s.match.arena.gateSpan()/2 + zoneMargin
	}
	return s.player.SpawnPos, zoneDefaultR
}

// broadcastZone sends the current arena's safe zone in the waiting stage. Called at a
// city-spawn teleport (round transition) so the client renders the NEW city's circle
// right away — otherwise the previous round's circle (at the old city's coords) stays up
// until the Fight phase's runSafeZone re-sends it, leaving a wrong/absent zone during the
// buy phase at the new city.
func (s *session) broadcastZone() {
	center, outerR := s.zoneGeometry()
	innerR := outerR * zoneInnerRatio
	s.sendZone(byte(s.match.round), center, outerR, center, innerR, message.ZoneWaiting, zoneWaitDur)
}

// sendZone emits one cmd 117 zone update, StartMs=now and EndMs=now+dur on the match
// clock (our approximation of the client GameTime).
func (s *session) sendZone(stage byte, outer message.Vec3, outerR float64, inner message.Vec3, innerR float64, timeSpan byte, dur time.Duration) {
	nowMs := uint32(time.Since(s.match.matchStart).Milliseconds())
	endMs := nowMs + uint32(dur.Milliseconds())
	body := message.SafeZoneChange(stage, outer, outerR, inner, innerR, timeSpan, nowMs, endMs)
	s.sendDataLog(packet.CmdSafeZoneChange, body,
		fmt.Sprintf("cmd=117 SafeZone stage=%d span=%d r=%.0f->%.0f center=(%.0f,%.0f) end=%dms",
			stage, timeSpan, outerR, innerR, outer.X, outer.Z, endMs))
}

// lerpRadius interpolates the zone radius from outer to inner over zoneShrinkDur.
func lerpRadius(outer, inner float64, elapsed time.Duration) float64 {
	t := elapsed.Seconds() / zoneShrinkDur.Seconds()
	if t <= 0 {
		return outer
	}
	if t >= 1 {
		return inner
	}
	return outer + (inner-outer)*t
}

// dist2D returns the horizontal distance between two world points.
func dist2D(a, b message.Vec3) float64 { return math.Hypot(a.X-b.X, a.Z-b.Z) }
