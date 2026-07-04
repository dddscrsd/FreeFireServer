package main

import (
	"encoding/binary"
	"fmt"
	"log"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// HP system: each tracked entity (local player, bot) has a current HP that damage
// reports (cmd 106) decrement and the PRI stream (cmd 900) replicates, so the client
// sees an enemy's HP drop toward a kill. Death (cmd 107) is not sent yet.

// initHP seeds every tracked entity to full HP. Called once when the CS sync loop
// starts, before the PRI stream begins replicating HP.
func (s *session) initHP() {
	s.hpMu.Lock()
	s.hp = map[uint32]uint16{playerEntityID: maxHP, s.botEntity: maxHP}
	s.playerPos = s.player.SpawnPos // seed the tracked pos to the spawn (before the first cmd 1001)
	s.hpMu.Unlock()
}

// entityHP returns an entity's current HP (maxHP if untracked / not yet seeded).
func (s *session) entityHP(entity uint32) uint16 {
	s.hpMu.Lock()
	defer s.hpMu.Unlock()
	if hp, ok := s.hp[entity]; ok {
		return hp
	}
	return maxHP
}

// handleTakeDamage handles cmd 106 (RUDP_TAKE_DAMAGE): the shooter's client reports a
// hit. The (large) hit report leads with [victim u32][damage u16][.. attacker u32 @10]
// — the rest is hit context (position, weapon, hit part) we don't need. The damage is
// the client's already-calculated net value (body-part multiplier + armour applied),
// so it applies straight to HP. We subtract it from the victim's tracked HP; the PRI
// stream replicates the new HP, and a transition to 0 triggers the death packet.
func (s *session) handleTakeDamage(p *packet.Packet) {
	if len(p.Payload) < 14 {
		log.Printf("[mm-udp] cmd=106 take-damage too short (%dB): %x", len(p.Payload), p.Payload)
		return
	}
	victim := binary.LittleEndian.Uint32(p.Payload[0:])
	dmg := binary.LittleEndian.Uint16(p.Payload[4:])
	base_dmg := binary.LittleEndian.Uint16(p.Payload[6:])
	if dmg < base_dmg {
		return
	}
	// Hit body part is at offset 9 — the high byte of the packed field @6, which is
	// (rawBaseDamage & 0xFFFFFF) | (bodyPart<<24); it also appears at the dedicated byte
	// @22. Offset 8 (what we read before) is raw-damage bits 16-23, ~always 0, so it
	// never showed a headshot. EColliderType: 1=HEAD, 2=BODY, 3=LIMB, 4=VEHICLE, 5=PROT.
	bodyPart := uint8(p.Payload[9])
	attacker := binary.LittleEndian.Uint32(p.Payload[10:])

	s.applyDamage(victim, attacker, dmg, bodyPart)
}

// applyDamage subtracts dmg from the victim's HP (clamped at 0) and logs it. On the
// transition to 0 HP it sends the death packet (once — a victim already at 0 is
// ignored). Untracked entities are ignored.
func (s *session) applyDamage(victim, attacker uint32, dmg uint16, bodyPart uint8) {
	s.hpMu.Lock()
	cur, ok := s.hp[victim]
	if !ok || cur == 0 { // untracked, or already dead (don't re-kill)
		s.hpMu.Unlock()
		return
	}
	if dmg >= cur {
		cur = 0
	} else {
		cur -= dmg
	}
	s.hp[victim] = cur
	s.hpMu.Unlock()

	log.Printf("[mm-udp] TAKE_DAMAGE victim=%#x dmg=%d -> HP=%d", victim, dmg, cur)
	if cur == 0 {
		s.killEntity(victim, attacker, bodyPart)
	}
}

// killCauseDamageZone is the cmd 107 weaponDataID for an out-of-zone environment death
// (message enum KILL_BY_DAMAGEZONE). With killer=0 the client books it as a no-killer
// environment elimination — and because -20 is a RECOGNISED cause it is NOT the
// "backend/system kill" (that is the unrecognised-negative default). See
// [[bot-networkaipawn]].
const killCauseDamageZone = -20

// killEntity sends the death packet (cmd 107) marking victim eliminated. A player kill
// (killer=local player) is tagged with the held weapon for the kill feed; killer=0 is
// an out-of-zone environment death (no killer, cause KILL_BY_DAMAGEZONE, not a system
// kill). When the LOCAL player dies it also carries an ObserveID (a living entity to
// spectate) so the client stays in the match instead of returning to the lobby.
func (s *session) killEntity(victim, killer uint32, bodyPart uint8) {
	if killer == playerEntityID && victim == s.botEntity {
		s.roundKills.Add(1) // local player killed the bot — counts toward the round coin award
	}
	var weapon int32
	var weaponSkin int32
	switch killer {
	case playerEntityID: // only the local player's loadout is tracked so far
		if (s.heldWeaponData()) == 0 {
			weapon = 1 // fists
		} else {
			weapon = int32(s.heldWeaponData()) // the held weapon's ID
		}
		weaponSkin = int32(SkinForWeapon(uint32(weapon), []uint32(s.player.Slots)))
	case 0: // out-of-zone environment death
		weapon = killCauseDamageZone
		weaponSkin = 0
	}
	var observe uint32
	if victim == playerEntityID { // the dead local client must be given something to spectate
		observe = s.spectateTarget()
	}
	// Mark the LOCAL player's non-deciding death as PENDING REVIVE so the client keeps the
	// pawn (down, not removed) — otherwise a full death destroys it and the round-start
	// cmd 388 revive has nothing to resurrect. The bot (and the deciding death, observe=0)
	// stay full deaths.
	pendingRevive := victim == playerEntityID
	body := message.PlayerDead(victim, killer, uint32(weapon), uint32(weaponSkin), observe, bodyPart, s.deathPos(victim), pendingRevive)
	s.sendDataLog(packet.CmdDead, body,
		fmt.Sprintf("cmd=107 DEAD victim=%#x killer=%#x weapon=%d weaponSkin=%d observe=%#x bodyPart=%d pending=%v", victim, killer, weapon, weaponSkin, observe, bodyPart, pendingRevive))
}

// spectateTarget returns the ObserveID (spectate focus) for a dead local player. It is
// the player's OWN id — the client watches its own corpse (the normal CS post-death
// state). This is REQUIRED for the cmd 388 revive to un-spectate cleanly: the client's
// full spectator teardown (camera to self, IsObserver=0) only fires when the observer was
// targeting the revived id, i.e. the player itself. Pointing it at the bot instead left
// the player stuck spectating after the revive. It returns 0 (no spectate -> client
// bails) when this death hands the enemy the DECIDING round, so the MatchEnd (cmd 103)
// screen follows instead.
func (s *session) spectateTarget() uint32 {
	if int(s.teamScore[1])+1 >= roundsToWin { // this death makes the enemy reach the win target
		return 0
	}
	if s.entityHP(s.botEntity) > 0 {
		return s.botEntity // bot is alive, spectate it
	}
	return playerEntityID // watch own corpse; the round-transition cmd 388 revive un-spectates it
}

// handleChangeHeldItem handles cmd 108 (RUDP_CHANGE_INVENTORY_ON_HAND): the client
// reports which item its player now holds, as [entity u32][itemUnique u32]. Tracking
// the local player's in-hand item keeps heldWeaponData (and thus the kill-feed weapon)
// correct after a mid-match weapon switch (e.g. switching to fists).
func (s *session) handleChangeHeldItem(p *packet.Packet) {
	if len(p.Payload) < 8 {
		return
	}
	entity := binary.LittleEndian.Uint32(p.Payload[0:])
	unique := binary.LittleEndian.Uint32(p.Payload[4:])
	if entity != playerEntityID {
		return // only track our local player's held item
	}
	s.invMu.Lock()
	s.itemOnHand = unique
	s.invMu.Unlock()
}

// heldWeaponData returns the DataID of the item the local player currently holds (the
// equipment slot whose unique matches itemOnHand), or 0 (e.g. fists).
func (s *session) heldWeaponData() uint32 {
	s.invMu.Lock()
	defer s.invMu.Unlock()
	for _, e := range s.equipment {
		if e.Unique == s.itemOnHand {
			return e.Data
		}
	}
	return 0
}

// deathPos returns the world position to report an entity died at: the local player's
// last-reported position (it moves — e.g. a zone death happens where it wandered), or
// the bot's static spawn.
func (s *session) deathPos(entity uint32) message.Vec3 {
	switch entity {
	case s.botEntity:
		return s.bot.SpawnPos
	case playerEntityID:
		s.hpMu.Lock()
		pos := s.playerPos
		s.hpMu.Unlock()
		if pos != (message.Vec3{}) {
			return pos
		}
		return s.player.SpawnPos
	}
	return message.Vec3{}
}
