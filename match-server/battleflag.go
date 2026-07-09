package main

import (
	"fmt"
	"log"
	"time"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// Named battle-flag world objects, planted by the flag emotes (909000034 Pirate's / 909000128 Ruler's).
// RE (see [[cs-emotes]]): the client only plants when it has a battle flag EQUIPPED (Player.BattleFlagID
// != 0, delivered in cmd 101). It plays the emote (cmd 369, relayed by emote.go), and at the plant keyframe
// self-spawns a LOCAL ground flag + sends cmd 0x114 RequestUseBattleFlag with the plant position + the flag
// config resolved from the emote's link_id. The server is authoritative for the REPLICATED entity: it
// assigns a fresh object id and broadcasts cmd 0x115 to ALL — the owner's client adopts its pre-spawned
// ground flag, remotes spawn fresh from the prefab, and every client renders the owner's NAME overhead
// (resolved from the owner id; no name is ever sent). ONE flag per player: re-planting despawns the
// previous one (active=false) and spawns the new one in the SAME cmd 0x115 list (atomic replace). There is
// NO client-side lifetime timer, so the server despawns each flag after flagLifetime (BattleFlag.csv 60s).

const flagEntityBase uint32 = 0x04000000 // flag object-id band, disjoint from players (0x01) / bot (0x02) / walls (0x03) / containers (u16)

// flagLifetime is how long a planted flag lives before the server despawns it (BattleFlag.csv duration=60s;
// the client holds it indefinitely otherwise).
const flagLifetime = 60 * time.Second

// flagObject tracks one player's live flag: its object/replication id + when it was planted (lifetime).
type flagObject struct {
	id   uint32
	born time.Time
}

// nextFlagID allocates a fresh flag object/replication id (monotonic, never reused).
func (m *Match) nextFlagID() uint32 {
	if m.flags == nil {
		m.flags = map[uint32]flagObject{}
	}
	m.flagSeq++
	return flagEntityBase | m.flagSeq
}

// handleBattleFlagReq handles cmd 0x114 (RequestUseBattleFlag): the player planted a flag emote's flag.
// It replaces the player's existing flag (if any) with a fresh one at the plant position, broadcasting the
// despawn+spawn atomically as one cmd 0x115 list so every client sees the swap.
func (s *session) handleBattleFlagReq(p *packet.Packet) {
	req, ok := message.ParseBattleFlagReq(p.Payload)
	if !ok {
		log.Printf("[mm-udp] cmd=276 battleflag req too short (%dB): %x", len(p.Payload), p.Payload)
		return
	}
	m := s.match
	owner := s.player.EntityID // authoritative owner (ignore the client-supplied id so the name resolves)
	if m.flags == nil {
		m.flags = map[uint32]flagObject{}
	}

	var entries []message.BattleFlagEntry
	if old, exists := m.flags[owner]; exists { // 1 per player: despawn the previous flag first (same packet)
		entries = append(entries, message.BattleFlagEntry{Owner: owner, ObjectId: old.id, Active: false})
	}
	newID := m.nextFlagID()
	entries = append(entries, message.BattleFlagEntry{Owner: owner, ObjectId: newID, Active: true, Pos: req.Pos, ConfigId: req.ConfigId})
	m.flags[owner] = flagObject{id: newID, born: time.Now()}

	m.broadcastData(packet.CmdBattleFlagSync, message.BattleFlagSync(entries),
		fmt.Sprintf("cmd=277 battleflag owner=%#x obj=%d cfg=%d pos=(%.1f,%.1f,%.1f) replace=%v -> all",
			owner, newID, req.ConfigId, req.Pos.X, req.Pos.Y, req.Pos.Z, len(entries) > 1))
}

// stepFlags despawns flags that have outlived flagLifetime — the client holds a flag until the server stops
// replicating it, so the 60s duration is server-enforced. Run each match tick.
func (m *Match) stepFlags(now time.Time) {
	for owner, fo := range m.flags {
		if now.Sub(fo.born) >= flagLifetime {
			delete(m.flags, owner)
			m.broadcastData(packet.CmdBattleFlagSync,
				message.BattleFlagSync([]message.BattleFlagEntry{{Owner: owner, ObjectId: fo.id, Active: false}}),
				fmt.Sprintf("cmd=277 battleflag despawn owner=%#x obj=%d (%.0fs lifetime)", owner, fo.id, flagLifetime.Seconds()))
		}
	}
}

// clearFlag despawns a single player's flag (if any) and drops the tracking — used on player disconnect.
func (m *Match) clearFlag(owner uint32) {
	fo, exists := m.flags[owner]
	if !exists {
		return
	}
	delete(m.flags, owner)
	m.broadcastData(packet.CmdBattleFlagSync,
		message.BattleFlagSync([]message.BattleFlagEntry{{Owner: owner, ObjectId: fo.id, Active: false}}),
		fmt.Sprintf("cmd=277 battleflag despawn owner=%#x obj=%d (quit)", owner, fo.id))
}

// clearAllFlags despawns every live flag and empties the map — flags are per-round world state (the round
// resets to a new arena), so this runs on a round transition, like clearWalls.
func (m *Match) clearAllFlags() {
	if len(m.flags) == 0 {
		return
	}
	entries := make([]message.BattleFlagEntry, 0, len(m.flags))
	for owner, fo := range m.flags {
		entries = append(entries, message.BattleFlagEntry{Owner: owner, ObjectId: fo.id, Active: false})
	}
	m.flags = map[uint32]flagObject{}
	m.broadcastData(packet.CmdBattleFlagSync, message.BattleFlagSync(entries),
		fmt.Sprintf("cmd=277 clear %d flags (round reset)", len(entries)))
}
