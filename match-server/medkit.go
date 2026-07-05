package main

import (
	"log"
	"time"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// Consumable use / medkit heal-over-time. The client channels a consumable: it sends
// cmd 131 RUDP_TRYUSE_INVENTORY when the use animation STARTS and cmd 113
// RUDP_USE_INVENTORY when it FINISHES (decrementing its own inventory copy locally). On
// the finishing cmd 113 the server consumes the item from ITS tracking and, for a medkit,
// heals the player 75 HP gradually — each
// step streamed as a minimal HP-only PRI (cmd 900). Modelled on the reference server.
const (
	medkitData     = 102
	medkitSteps    = 75
	medkitPerStep  = 1
	medkitInterval = 5 * time.Second / medkitSteps
)

// handleTryUseInventory handles cmd 131 (KOMODKGGDBG, [u32 entity][u32 item][u32 tick]):
// the client STARTED channelling a consumable. The heal + inventory consume happen on the
// finishing cmd 113, so this only records the start for the log.
func (s *session) handleTryUseInventory(p *packet.Packet) {
	log.Printf("[mm-udp] cmd=131 TRYUSE_INVENTORY (start) %x", p.Payload)
}

// handleUseInventory handles cmd 113: the client FINISHED using a consumable. It finds the
// item by UniqueID, consumes one from the server's inventory tracking (the client already
// decremented its own copy, so no cmd 174 is sent), and for a medkit starts the gradual
// heal. Consumables are kept at count 0 rather than deleted, matching the client's
// UniqueID-reuse behaviour on the next pickup of the same DataID.
func (s *session) handleUseInventory(p *packet.Packet) {
	req, ok := message.ParseUseInventory(p.Payload)
	if !ok {
		log.Printf("[mm-udp] cmd=113 use req too short (%dB): %x", len(p.Payload), p.Payload)
		return
	}

	s.invMu.Lock()
	it, held := s.clientUIDs[req.Unique]
	if !held {
		s.invMu.Unlock()
		log.Printf("[mm-udp] cmd=113 use unknown uid=%d %x", req.Unique, p.Payload)
		return
	}
	if it.count > 0 {
		it.count-- // keep the item even at 0 (consumable UniqueID reuse)
		s.clientUIDs[req.Unique] = it
	}
	data, remaining := it.data, it.count
	s.invMu.Unlock()

	log.Printf("[mm-udp] cmd=113 USE_INVENTORY uid=%d data=%d -> %d left", req.Unique, data, remaining)

	if data == medkitData {
		go s.healMedkit()
	}
}

// healMedkit restores 80 HP to the local player gradually — medkitSteps ticks of
// medkitPerStep HP, medkitInterval apart — streaming each new HP as a minimal cmd 900 PRI
// (only the CUR_HP field) and updating s.hp so the regular PRI stream and the round/death
// logic stay consistent. Stops early if the player dies (hp reaches 0), hits full HP, or
// the session ends.
func (s *session) healMedkit() {
	for i := 0; i < medkitSteps; i++ {
		time.Sleep(medkitInterval)
		if s.stopped.Load() {
			return
		}
		s.hpMu.Lock()
		hp := s.hp[playerEntityID]
		if hp == 0 { // don't revive a dead player via a lingering heal
			s.hpMu.Unlock()
			return
		}
		hp += medkitPerStep
		if hp > maxHP {
			hp = maxHP
		}
		s.hp[playerEntityID] = hp
		s.hpMu.Unlock()

		s.sendVar(packet.CmdPRISync, message.PRIHealHP(playerRepID, hp), 1)
		if hp >= maxHP {
			return
		}
	}
}
