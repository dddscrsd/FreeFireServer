package main

import (
	"math"
	"math/rand"
	"time"

	"libmadoka/match-server/message"
)

// Contra Squad spawn arenas. Each "city" has TWO gate fences on opposite sides of
// that location; the two teams spawn at the SAME city but at DIFFERENT fences, so
// they start apart and facing each other. Positions are Unity world-space meters,
// dumped live from the QualityObject fence transforms via LibTool — see
// protocol/contra_squad_fences_positions.txt (Fence0 = "<city> #1", Fence1 = "#2").
// Fence rotations were not mapped, so each team's facing is DERIVED here: point the
// horizontal vector from your fence toward the opposing fence.
//
// ============================ ROUND SYSTEM TODO ============================
// The CS round loop is NOT implemented yet. The user wants a NEW random city
// picked at the start of EVERY round (not only at match start). When the round
// system lands:
//   1. Call pickArena() again at each round start.
//   2. Reposition BOTH teams to the new city's fences. Mid-match the players
//      already exist, so cmd 101 (join-only spawn) no longer applies — use the
//      JIIKBLKJCKM teleport lever (message.ForceTeleport, cmd 145) instead. Note
//      it must be STREAMED briefly to hold the autonomous local player, or sent
//      during a respawn/frozen state — see [[cs-spawn-teleport-timer]].
//   3. In a real multi-client match, pick ONE arena per round on the SERVER and
//      give every player that same city (this file currently picks per-join,
//      which is fine for the single-player test but would desync real teams).
// ==========================================================================

type csArena struct {
	City   string
	ZoneID uint16
	Fence0 message.Vec3 // faction 0 (left) gate
	Fence1 message.Vec3 // faction 1 (right) gate
}

func v(x, y, z float64) message.Vec3 { return message.Vec3{X: x, Y: y, Z: z} }

// csArenas: 12 cities, each a pair of opposing gate fences (world meters).
var csArenas = []csArena{
	{"factory", 0, v(366.6, 15.9, -261.5), v(477.7, 15.9, -250.6)},
	{"clock tower", 1, v(158.8, 0.4, -148.4), v(295.3, 2.5, -85.6)},
	{"observatory", 2, v(-146.4, 30.0, 320.5), v(-122.3, 27.4, 408.8)},
	{"katulistiwa", 3, v(131.0, 2.1, 415.6), v(197.8, 1.4, 278.6)},
	{"shipyard", 4, v(306.0, 9.7, 769.7), v(420.8, 9.6, 704.9)},
	{"mars electric", 5, v(565.5, 24.7, -560.1), v(467.1, 24.7, -508.5)},
	{"pochinok", 6, v(659.5, 25.1, -227.0), v(645.6, 25.1, -335.4)},
	{"sentosa", 7, v(1188.6, 5.6, -376.0), v(1162.8, 6.0, -482.0)},
	{"cape town", 8, v(1228.0, 0.2, 116.3), v(1336.4, 0.2, 128.0)},
	{"mill", 9, v(909.9, 49.1, 450.5), v(906.9, 47.3, 546.4)},
	{"hangar", 10, v(-33.8, -0.3, 39.7), v(43.8, -0.4, 121.0)},
	{"riverside", 11, v(601.9, 14.2, 494.5), v(509.6, 14.2, 494.5)},
}

var spawnRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// pickArena selects a random CS spawn city. Called at match start (and, once the
// round loop exists, at each round start — see the ROUND SYSTEM TODO above).
func pickArena() csArena { return csArenas[spawnRand.Intn(len(csArenas))] }

// spawnFor returns the (position, facing) for a faction in this arena: faction 0
// spawns at Fence0, faction 1 at Fence1, each facing the opposing fence.
func (a csArena) spawnFor(faction byte) (pos, face message.Vec3) {
	if faction == 1 {
		return a.Fence1, faceToward(a.Fence1, a.Fence0)
	}
	return a.Fence0, faceToward(a.Fence0, a.Fence1)
}

// center returns the midpoint of the two gates — the arena's play-area centre, used
// as the SafeZone circle centre.
func (a csArena) center() message.Vec3 {
	return message.Vec3{
		X: (a.Fence0.X + a.Fence1.X) / 2,
		Y: (a.Fence0.Y + a.Fence1.Y) / 2,
		Z: (a.Fence0.Z + a.Fence1.Z) / 2,
	}
}

// gateSpan returns the horizontal distance between the two gate fences — the scale of
// the play area, used to size the SafeZone radius.
func (a csArena) gateSpan() float64 {
	return math.Hypot(a.Fence1.X-a.Fence0.X, a.Fence1.Z-a.Fence0.Z)
}

// faceToward returns the horizontal unit vector from `from` toward `to` (y=0).
func faceToward(from, to message.Vec3) message.Vec3 {
	dx, dz := to.X-from.X, to.Z-from.Z
	d := math.Hypot(dx, dz)
	if d == 0 {
		return message.Vec3{X: 0, Y: 0, Z: 1}
	}
	return message.Vec3{X: dx / d, Y: 0, Z: dz / d}
}
