package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"time"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// Consumable use / medkit heal-over-time. The client channels a consumable: it sends cmd 131
// RUDP_TRYUSE_INVENTORY when the use animation STARTS and cmd 113 RUDP_USE_INVENTORY when it
// FINISHES (decrementing its own inventory copy locally). On the finishing cmd 113 the server
// consumes the item from ITS tracking and, for a medkit, ARMS a heal-over-time that the match
// loop applies each tick (stepHeal) — 75 HP over 5s, streamed as minimal HP-only PRIs (cmd 900).
// Modelled on the reference server. Was a per-heal goroutine; now run()-driven so there is a
// single owner of hp (Step 3b).
const (
	medkitData      = 102
	medkitSteps     = 75
	medkitPerStep   = 1
	medkitInterval  = 5 * time.Second / medkitSteps // per-HP spacing; medkitSteps of these == 5s total
	medkitChannelMs = 4000                          // cure-channel anim length hint (ms) sent on the remote DoAction Start; the finishing cmd 113 sends End, which is the real terminator
	// medkitChannelTimeout bounds how long a cure animation may play without a finishing cmd 113 before
	// stepHealChannel force-ends it — covers a move/abort cancel the client never reports. Kept generously
	// above the ~4s channel so a legitimately slow heal is never cut short (death/knock ends it sooner).
	medkitChannelTimeout = 6 * time.Second
)

// handleTryUseInventory handles cmd 131 (KOMODKGGDBG, [u32 entity][u32 item][u32 tick]): the client
// STARTED channelling a consumable. It IMMEDIATELY broadcasts a cmd 201 DoAction "Start" so REMOTE pawns
// begin the cure channel animation right away (the client's own cmd 131 only fires a local HUD event and
// never animates other players — the server is the only thing that can make the heal visible to them, and
// it must be at the START, not when the heal finishes). In CS the medkit is the only channelled consumable,
// so every cmd 131 is a cure. The channel is closed by the finishing cmd 113, or by stepHealChannel on an
// interrupt/timeout.
func (s *session) handleTryUseInventory(p *packet.Packet) {
	if len(p.Payload) < 8 {
		log.Printf("[mm-udp] cmd=131 TRYUSE_INVENTORY (start) short %x", p.Payload)
		return
	}
	item := binary.LittleEndian.Uint32(p.Payload[4:]) // [u32 entity][u32 item][u32 tick] — logged for diagnostics
	s.healChanneling = true
	s.healChannelStart = time.Now()
	s.match.broadcastData(packet.CmdDoAction,
		message.DoAction(s.player.EntityID, message.DoActionStart, message.ActionUseMedkit, medkitChannelMs, true),
		fmt.Sprintf("cmd=201 DoAction medkit cure START ent=%#x item=%d -> all (remotes animate)", s.entityID, item))
}

// endHealChannel stops the remote cure animation (cmd 201 DoAction End) if a medkit channel is active.
// Idempotent — safe to call on the finishing cmd 113, on interruption (death/knock/timeout), and repeatedly.
func (s *session) endHealChannel() {
	if !s.healChanneling {
		return
	}
	s.healChanneling = false
	s.match.broadcastData(packet.CmdDoAction,
		message.DoAction(s.player.EntityID, message.DoActionEnd, message.ActionUseMedkit, 0, true),
		fmt.Sprintf("cmd=201 DoAction medkit cure END ent=%#x -> all", s.entityID))
}

// stepHealChannel force-ends a stuck cure animation on each match tick: if the channelling player was
// interrupted (death/knock → life != alive) or the channel ran past its window without a finishing cmd
// 113 (the player moved/aborted the heal, which the client never reports), broadcast the DoAction End so
// the remote animation doesn't loop forever.
func (s *session) stepHealChannel(now time.Time) {
	if !s.healChanneling {
		return
	}
	if s.match.lifeOf(s.entityID) != lifeAlive || now.Sub(s.healChannelStart) > medkitChannelTimeout {
		s.endHealChannel()
	}
}

// handleUseInventory handles cmd 113: the client FINISHED using a consumable. It finds the item
// by UniqueID, consumes one from the server's inventory tracking (the client already decremented
// its own copy, so no cmd 174 is sent), and for a medkit ARMS the heal-over-time (stepHeal drives
// it from the match loop). Consumables are kept at count 0 rather than deleted, matching the
// client's UniqueID-reuse behaviour on the next pickup of the same DataID. Runs on run()'s
// goroutine (via the mailbox), so it mutates inventory + heal state without a lock.
func (s *session) handleUseInventory(p *packet.Packet) {
	req, ok := message.ParseUseInventory(p.Payload)
	if !ok {
		log.Printf("[mm-udp] cmd=113 use req too short (%dB): %x", len(p.Payload), p.Payload)
		return
	}

	it, held := s.clientUIDs[req.Unique]
	if !held {
		log.Printf("[mm-udp] cmd=113 use unknown uid=%d %x", req.Unique, p.Payload)
		return
	}
	if it.count > 0 {
		it.count-- // keep the item even at 0 (consumable UniqueID reuse)
		s.clientUIDs[req.Unique] = it
	}
	data, remaining := it.data, it.count

	log.Printf("[mm-udp] cmd=113 USE_INVENTORY uid=%d data=%d -> %d left", req.Unique, data, remaining)

	// The consumable channel finished — close the remote cure animation (idempotent; opened on cmd 131).
	s.endHealChannel()

	if data == medkitData {
		s.healActive = true
		s.healStart = time.Now()
		s.healApplied = 0
	}
}

// stepHeal applies the medkit heal-over-time on each match tick (run()-driven, replacing the old
// per-heal goroutine): it tops the local player up to the HP that should have accrued by `now`
// since the heal was armed (+medkitPerStep every medkitInterval, up to medkitSteps), streaming
// the new HP as a minimal cmd 900 PRI whenever it changes so the heal looks smooth. Stops at full
// HP, on death (never revive a dead player via a lingering heal), or once all steps are applied.
func (s *session) stepHeal(now time.Time) {
	if !s.healActive {
		return
	}
	m := s.match
	if m.lifeOf(s.entityID) != lifeAlive { // dead or KNOCKED (bleed pool != 0) -> the heal is interrupted
		s.healActive = false
		return
	}
	hp := m.hp[s.entityID]
	want := int(now.Sub(s.healStart) / medkitInterval)
	if want > medkitSteps {
		want = medkitSteps
	}
	if want > s.healApplied {
		hp += uint16((want - s.healApplied) * medkitPerStep)
		if hp > m.settings.maxHP {
			hp = m.settings.maxHP
		}
		m.hp[s.entityID] = hp
		s.healApplied = want
		s.sendVar(packet.CmdPRISync, message.PRIHealHP(s.repID, hp), 1)
	}
	if s.healApplied >= medkitSteps || hp >= m.settings.maxHP {
		s.healActive = false
	}
}
