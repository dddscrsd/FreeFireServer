package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// config holds server tunables plus the optional MATCH_* diagnostic knobs, read
// once at startup so the hot paths (handlers, csSyncLoop) don't scatter os.Getenv
// calls. cfg is the process-wide instance, set by main().
type config struct {
	jwtSecret []byte        // verifies the cmd 440 prepare_token (env MATCH_JWT_SECRET)
	buyPhase  time.Duration // CS Prepare/shop duration before advancing to Fight

	// Diagnostics — all optional; the zero value is production behaviour.
	holdPrepare bool          // MATCH_HOLD_PREPARE=1: never advance past Prepare (keeps spawn fences up)
	spawn       *message.Vec3 // MATCH_SPAWN="x,y,z": force this spawn instead of a random arena
	spawnFace   message.Vec3  // MATCH_SPAWN_FACE="x,y,z" (default +Z)
	teleport    *teleportTest // MATCH_TELEPORT="x,y,z": stream a force-teleport at the player (nil = off)
	debugPos    bool          // MATCH_DEBUG_POS=1: log every parsed client position (cmd 1001)
	zoneTest    bool          // MATCH_ZONE_TEST=1: offset the zone 100m + short wait so the player dies to it fast (spectate/death testing)
}

var cfg config

// loadConfig reads the environment into a config. Called once from main().
func loadConfig() config {
	c := config{
		jwtSecret:   []byte(envOr("MATCH_JWT_SECRET", "dev-match-secret-change-me")),
		buyPhase:    10 * time.Second,
		spawnFace:   message.Vec3{X: 0, Y: 0, Z: 1},
		holdPrepare: os.Getenv("MATCH_HOLD_PREPARE") == "1",
		debugPos:    os.Getenv("MATCH_DEBUG_POS") == "1",
		zoneTest:    os.Getenv("MATCH_ZONE_TEST") == "1",
	}
	if sp, ok := parseVec3(os.Getenv("MATCH_SPAWN")); ok {
		c.spawn = &sp
		if f, ok := parseVec3(os.Getenv("MATCH_SPAWN_FACE")); ok {
			c.spawnFace = f
		}
	}
	if tp, ok := loadTeleportTest(); ok {
		c.teleport = &tp
	}
	return c
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// parseVec3 parses "x,y,z" (floats) into a message.Vec3.
func parseVec3(s string) (message.Vec3, bool) {
	var v message.Vec3
	if n, err := fmt.Sscanf(s, "%f,%f,%f", &v.X, &v.Y, &v.Z); err != nil || n != 3 {
		return v, false
	}
	return v, true
}

// teleportTest is the MATCH_TELEPORT diagnostic: stream a JIIKBLKJCKM force-teleport
// at the joining player. Kept as the live harness for the mid-match reposition lever
// the round system will use (see spawns.go ROUND SYSTEM TODO). Overrides:
// MATCH_TELEPORT_FACE, MATCH_TELEPORT_CMD (144|145, default 145=SyncTeleportInfo,
// which also sets rotation), MATCH_TELEPORT_PHYS.
type teleportTest struct {
	pos, face message.Vec3
	cmd       uint16
	phys      byte
}

func loadTeleportTest() (teleportTest, bool) {
	pos, ok := parseVec3(os.Getenv("MATCH_TELEPORT"))
	if !ok {
		return teleportTest{}, false
	}
	t := teleportTest{pos: pos, face: message.Vec3{X: 0, Y: 0, Z: 1}, cmd: packet.CmdTeleport}
	if f, ok := parseVec3(os.Getenv("MATCH_TELEPORT_FACE")); ok {
		t.face = f
	}
	if os.Getenv("MATCH_TELEPORT_CMD") == "144" {
		t.cmd = packet.CmdForceSync
	}
	if p, err := strconv.Atoi(os.Getenv("MATCH_TELEPORT_PHYS")); err == nil {
		t.phys = byte(p)
	}
	return t, true
}
