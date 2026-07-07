package main

// GLOO WALL (ice wall) server logic for Contra Squad. A player buys a gloo wall
// (item DataID 1201, equipped in SlotBuilding=13) and PLACES it: the client sends a
// cmd 218 RUDP_ADD_ICEWALL place request (message::JEGADMHBFKB); the server spawns an
// authoritative wall, broadcasts it back (cmd 218 KACGKFMOHNP), and streams its state.
//
// Each wall has 200 HP and a crack STATE (intact -> cracked -> broken). It takes damage
// via cmd 219 (client reports the delta); the server subtracts HP and, at 0, removes it
// (cmd 221). After ~60s a wall force-breaks regardless of HP/state (the lifetime sweep).
// Each wall owns its OWN PRI (RepID == its game id), streamed on the existing cmd 900 loop
// as a single byte (LevelIceWall field 0 crack state). The client self-binds a server-spawned
// level object's PRI on OnStart using get_UniqueID() (== the game id) as the ReplicationID and
// registers it in MatchGame.m_ReplicationEntitis under that id — the map the cmd 900 handler
// (DNCBMEDLPGP -> GetRepEntity) routes through. So the wall's PRI RepID MUST equal its game id;
// keying it on anything else means the crack-state block is silently skipped.
//
// Placing DEDUCTS one gloo wall from the player's inventory (clientUIDs data 1201) via a
// cmd 112 DropInventoryRes, UNLESS MATCH_INFINITE_GLOO is set. Even with infinite, the
// number of ACTIVE walls is capped at the player's current gloo-wall inventory count:
// placing beyond that removes the OLDEST placed wall (FIFO). The cap is read BEFORE the
// deduction so a single place trims at most one wall.

import (
	"fmt"
	"log"
	"time"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

const (
	glooDataID   uint32 = 1201             // gloo-wall item DataID (equips SlotBuilding=13)
	glooMaxHP    uint16 = 750              // fresh-wall HP (matches the client's spawn default)
	glooCrackHP  uint16 = 330              // <=44% of 750 -> cracked (tune here)
	glooLifetime        = 90 * time.Second // hard force-break regardless of HP/state
	glooWallCap         = 3                // max active walls per player (FIFO trim)

	// Wall entity/game id band, DISJOINT from player (0x01xxxxxx) + bot (0x02xxxxxx) + the
	// container id band. The wall's PRI RepID is the SAME value as its game id (see
	// nextWall): the client self-binds a level object's PRI under its UniqueID (= game id).
	wallEntityBase uint32 = 0x03000000 // wall entity/game id = wallEntityBase | seq (also the PRI RepID)
)

// Wall crack-state byte, as the client's LevelIceWall field-0 change handler
// (HPGILFDILLD @0x2c33e2c) reads it: the enum counts DOWN — 2 = intact, 1 = cracked
// (broken geometry + SOUND_ICEWALL_BROKEN), 0 = broken. The field's default is 2.
const (
	wallBroken  byte = 0
	wallCracked byte = 1
	wallIntact  byte = 2
)

// iceWall is one placed gloo wall (server-owned world object).
type iceWall struct {
	id       uint32       // wall entity game id (wallEntityBase band)
	repID    uint32       // per-wall PRI RepID (wallRepBase band)
	owner    uint32       // placer entity id (playerEntityID)
	pos      message.Vec3 // world metres
	rot      message.Quat // facing quaternion, echoed back opaquely
	wallType int32        // building/skin variant from the place request
	hp       uint16
	state    byte
	placedAt time.Time
}

// iceWallState builds the wire spawn/resync record (message::JJIODFAHEOG) for a wall.
// MovePlatformID/HealthExtra/Flag stay at their world-anchored defaults.
func (w *iceWall) iceWallState() message.IceWallState {
	return message.IceWallState{
		ID:       w.id,
		WallType: w.wallType,
		HP:       int32(w.hp),
		Pos:      w.pos,
		Rot:      w.rot,
		RepID:    w.repID,
	}
}

// nextWall allocates a fresh (id, RepID) pair, monotonic and never reused so a new
// wall can't collide with a stale one the client hasn't torn down.
func (s *session) nextWall() (id, repID uint32) {
	s.match.wallSeq++
	id = wallEntityBase | s.match.wallSeq
	// RepID == the game id: the client self-binds a level object's PRI on OnStart using
	// get_UniqueID() (== this id) as the ReplicationID, so the cmd 900 stream must key on it.
	return id, id
}

// wallByID returns the live wall with the given id, or nil.
func (s *session) wallByID(id uint32) *iceWall {
	for _, w := range s.match.walls {
		if w.id == id {
			return w
		}
	}
	return nil
}

// ownerWallCount counts an owner's live walls.
func (s *session) ownerWallCount(owner uint32) (n int) {
	for _, w := range s.match.walls {
		if w.owner == owner {
			n++
		}
	}
	return
}

// popOldestWall removes and returns the owner's oldest wall (walls is FIFO), or nil.
func (s *session) popOldestWall(owner uint32) *iceWall {
	for i, w := range s.match.walls {
		if w.owner == owner {
			s.match.walls = append(s.match.walls[:i], s.match.walls[i+1:]...)
			return w
		}
	}
	return nil
}

// removeWallByID drops the wall with the given id from the FIFO list.
func (s *session) removeWallByID(id uint32) {
	for i, w := range s.match.walls {
		if w.id == id {
			s.match.walls = append(s.match.walls[:i], s.match.walls[i+1:]...)
			return
		}
	}
}

// glooCount returns the (uid, count) of the player's gloo stack (data 1201), or (0,0).
func (s *session) glooCount() (uid, count uint32) {
	for u, it := range s.clientUIDs {
		if it.data == glooDataID {
			return u, it.count
		}
	}
	return 0, 0
}

// iceWallPRIBlock builds a wall's per-wall replication block: a single byte field 0 (the
// crack state). LevelIceWall::GetMaxRepDataCount is 1 — HP is NOT replicated, only this
// state byte. Streamed on the cmd 900 PRI loop keyed by the wall's own RepID.
func iceWallPRIBlock(state byte) []byte {
	return message.ReplicationBlock([]message.RepEntry{
		{Type: message.RepU8, Field: 0, Value: uint64(state)},
	})
}

// wallStateForHP maps current HP to the crack-state byte (counts DOWN, see the consts).
func wallStateForHP(hp uint16) byte {
	switch {
	case hp == 0:
		return wallBroken
	case hp <= glooCrackHP:
		return wallCracked
	default:
		return wallIntact
	}
}

// handlePlaceIcewall handles the cmd 218 client->server PLACE request. It deducts one
// gloo wall (unless MATCH_INFINITE_GLOO), spawns the wall (cmd 218 broadcast), and
// FIFO-trims the active walls down to the inventory-count cap (evicting the oldest via
// cmd 221). Packets are built into a slice then flushed in order (out/flush pattern).
func (s *session) handlePlaceIcewall(p *packet.Packet) {
	req, ok := message.ParsePlaceIcewallReq(p.Payload)
	if !ok {
		log.Printf("[mm-udp] cmd=218 place icewall req too short (%dB): %x", len(p.Payload), p.Payload)
		return
	}

	var out []outPkt

	glooUID, glooCount := s.glooCount()
	if glooCount == 0 { // holds no gloo -> nothing to place or cap against
		log.Printf("[mm-udp] cmd=218 place with 0 gloo in inventory — ignoring")
		return
	}

	// 1) DEDUCT one, unless infinite. cmd 112 DropInventoryRes SETS the client's stack to
	//    the new count (client removes the item at 0).
	if !cfg.infiniteGloo {
		newCount := glooCount - 1
		if newCount == 0 {
			delete(s.clientUIDs, glooUID)
			s.clearEquip(message.SlotBuilding) // free slot 13

		} else {
			it := s.clientUIDs[glooUID]
			it.count = newCount
			s.clientUIDs[glooUID] = it
		}
		out = append(out, outPkt{packet.CmdDropInventory,
			message.DropInventoryRes(glooUID, newCount, 0),
			fmt.Sprintf("cmd=112 gloo consume uid=%d -> %d left", glooUID, newCount)})
	}

	// 2) SPAWN the wall (cmd 218) with its game id as the ReplicationID, so the cmd 218
	//    self-bind (NBEKINCFDBB when RepID!=0) AND the client's own OnStart self-bind
	//    (get_UniqueID() == the game id) agree on the same RepID — the id the wall is
	//    registered under in MatchGame.m_ReplicationEntitis. Our cmd 900 crack-state stream is
	//    keyed by that same id (w.repID == w.id), so it routes to the wall. Trailing byte 0 =
	//    "not destroyed on spawn". No cmd 118 needed — the wall self-binds under its own id.
	id, repID := s.nextWall()
	w := &iceWall{id: id, repID: repID, owner: playerEntityID, pos: req.Pos, rot: req.Rot,
		wallType: req.WallType, hp: glooMaxHP, state: wallIntact, placedAt: time.Now()}
	s.match.walls = append(s.match.walls, w)
	out = append(out, outPkt{packet.CmdAddIcewall,
		message.AddIcewall(w.iceWallState(), 0),
		fmt.Sprintf("cmd=218 ADD_ICEWALL id=%d rep=%d hp=%d", w.id, w.repID, w.hp)})

	// 3) FIFO-TRIM active walls down to the cap (oldest first). One place normally trims
	//    at most one; the loop robustly enforces active <= cap.
	for s.ownerWallCount(playerEntityID) > glooWallCap {
		oldest := s.popOldestWall(playerEntityID)
		if oldest == nil {
			break
		}
		out = append(out, outPkt{packet.CmdRemoveIcewall, message.RemoveIcewall(oldest.id),
			fmt.Sprintf("cmd=221 REMOVE_ICEWALL id=%d (FIFO cap=%d)", oldest.id, glooWallCap)})
	}

	s.match.flushBroadcast(out)                           // flush the built batch (outPkt/flush pattern)
	s.sendVar(packet.CmdPRISync, s.match.priPayload(), 1) // eager wall-state PRI (don't wait ~300ms)
}

// handleIcewallDamage handles cmd 219 (client->server): the wall took damage. Subtract
// from its server-side HP; at 0 remove it (cmd 221), otherwise update its crack state
// (the cmd 900 PRI stream replicates the new state — no cmd 220 on crack, which would
// re-instantiate the wall client-side; cmd 220 is only for reconnect resync).
func (s *session) handleIcewallDamage(p *packet.Packet) {
	req, ok := message.ParseIcewallDamageReq(p.Payload)
	if !ok {
		return
	}
	var out []outPkt
	w := s.wallByID(req.WallID)
	if w == nil {
		return
	}
	dmg := uint16(req.Damage)
	if dmg == 0 { // a 0-damage report never no-ops
		log.Printf("[mm-udp] cmd=219 wall id=%d took 0 damage (weapon=%d) — forcing damage", w.id, req.Weapon)
		dmg = 25
	}
	if dmg >= w.hp {
		w.hp = 0
	} else {
		w.hp -= dmg
	}
	if w.hp == 0 { // broken -> remove
		s.removeWallByID(w.id)
		out = append(out, outPkt{packet.CmdRemoveIcewall, message.RemoveIcewall(w.id),
			fmt.Sprintf("cmd=221 REMOVE_ICEWALL id=%d (destroyed, weapon=%d)", w.id, req.Weapon)})
	} else {
		w.state = wallStateForHP(w.hp) // crack; the eager PRI below replicates it
		log.Printf("Updated wall state for wall id=%d", w.id)
	}

	s.flush(out)
	s.sendVar(packet.CmdPRISync, s.resyncWallsPri(), 1) // replicate the new HP/state on the PRI
}

// sweepWalls force-breaks any wall older than glooLifetime regardless of HP/state, and
// queues its cmd 221. Called on the streamPhase loop (~300ms) — 60s granularity is fine
// and it can't leak goroutines when the player quits or a round ends.
func (s *session) sweepWalls() {
	now := time.Now()
	var out []outPkt
	kept := s.match.walls[:0]
	for _, w := range s.match.walls {
		if now.Sub(w.placedAt) >= glooLifetime {
			out = append(out, outPkt{packet.CmdRemoveIcewall, message.RemoveIcewall(w.id),
				fmt.Sprintf("cmd=221 REMOVE_ICEWALL id=%d (lifetime)", w.id)})
		} else {
			kept = append(kept, w)
		}
	}
	s.match.walls = kept
	s.flush(out)
}

// clearWalls removes every live wall (cmd 221 each) and empties the list. Walls are
// per-round world state, so this is called on a round reset (roundTransition) so a stale
// wall doesn't survive the black-screen transition.
func (s *session) clearWalls() {
	var out []outPkt
	for _, w := range s.match.walls {
		out = append(out, outPkt{packet.CmdRemoveIcewall, message.RemoveIcewall(w.id),
			fmt.Sprintf("cmd=221 REMOVE_ICEWALL id=%d (round reset)", w.id)})
	}
	s.match.walls = nil
	s.flush(out)
}

// resyncWalls re-adds every live wall in one cmd 220 RESYNC_ICEWALL — used on a mid-match
// transport re-handshake (cmd 5) to redraw walls the fresh transport session lost.
func (s *session) resyncWalls() {
	states := make([]message.IceWallState, 0, len(s.match.walls))
	for _, w := range s.match.walls {
		states = append(states, w.iceWallState())
	}
	if len(states) == 0 {
		return
	}
	s.sendDataLog(packet.CmdResyncIcewall, message.ResyncIcewalls(states),
		fmt.Sprintf("cmd=220 RESYNC_ICEWALL (%d live walls)", len(states)))
}
