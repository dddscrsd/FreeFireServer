package main

import (
	"encoding/binary"
	"log"

	"libmadoka/match-server/packet"
)

// HP system: each tracked entity (local player, bot) has a current HP that damage
// reports (cmd 106) decrement and the PRI stream (cmd 900) replicates, so the client
// sees an enemy's HP drop toward a kill. Death (cmd 107) is not sent yet.

// initHP seeds every roster human + the bot to full HP and ALIVE and (re)initialises the life /
// knock maps. Called at match start, before the PRI stream begins replicating HP.
func (m *Match) initHP() {
	m.hp = map[uint32]uint16{}
	m.life = map[uint32]lifeState{}
	m.knock = map[uint32]*knockState{}
	m.kills = map[uint32]uint16{}
	m.deaths = map[uint32]uint16{}
	m.damage = map[uint32]uint32{}
	m.rescues = map[uint32]*rescueState{}
	for _, p := range m.players {
		m.hp[p.entityID] = m.settings.maxHP
		m.life[p.entityID] = lifeAlive
		p.playerPos = p.player.SpawnPos // seed the tracked pos to the spawn (before the first cmd 1001)
	}
	if m.botEntity != 0 {
		m.hp[m.botEntity] = m.settings.maxHP
		m.life[m.botEntity] = lifeAlive
	}
}

// entityHP returns an entity's current HP (the match's full HP if untracked / not yet seeded). It
// reads the match's shared HP map, so it is a Match method — any entity, human or bot, keyed by id.
func (m *Match) entityHP(entity uint32) uint16 {
	if hp, ok := m.hp[entity]; ok {
		return hp
	}
	return m.settings.maxHP
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
	// Hit body part is at offset 9 — the high byte of the packed field @6, which is
	// (rawBaseDamage & 0xFFFFFF) | (bodyPart<<24); it also appears at the dedicated byte
	// @22. Offset 8 (what we read before) is raw-damage bits 16-23, ~always 0, so it
	// never showed a headshot. EColliderType: 1=HEAD, 2=BODY, 3=LIMB, 4=VEHICLE, 5=PROT.
	bodyPart := uint8(p.Payload[9])
	attacker := binary.LittleEndian.Uint32(p.Payload[10:])

	s.match.applyDamage(victim, attacker, dmg, bodyPart)
}

// killCauseDamageZone is the cmd 107 weaponDataID for an out-of-zone environment death
// (message enum KILL_BY_DAMAGEZONE). With killer=0 the client books it as a no-killer
// environment elimination — and because -20 is a RECOGNISED cause it is NOT the
// "backend/system kill" (that is the unrecognised-negative default). See
// [[bot-networkaipawn]].
const killCauseDamageZone = -20

// handleChangeHeldItem handles cmd 108 (RUDP_CHANGE_INVENTORY_ON_HAND): the client
// reports which item its player now holds, as [entity u32][itemUnique u32]. Tracking
// the local player's in-hand item keeps heldWeaponData (and thus the kill-feed weapon)
// correct after a mid-match weapon switch (e.g. switching to fists).
func (s *session) handleChangeHeldItem(p *packet.Packet) {
	if len(p.Payload) < 8 {
		return
	}
	unique := binary.LittleEndian.Uint32(p.Payload[4:])
	// Remember the weapon held right before switching TO the gloo wall, so gloo.go can auto-switch back
	// to it once the last wall is thrown (the reference does this so the player isn't left empty-handed).
	if s.isGloo(unique) && !s.isGloo(s.itemOnHand) {
		s.heldBeforeGloo = s.itemOnHand
	}
	s.itemOnHand = unique
	s.sendVar(packet.CmdPRISync, s.match.priPayload(), 1) // eager: remote clients swap to the new held weapon (PRI field 4)
}

// isGloo reports whether an item unique is the player's gloo-wall stack (data glooDataID).
func (s *session) isGloo(unique uint32) bool {
	if it, ok := s.clientUIDs[unique]; ok {
		return it.data == glooDataID
	}
	return false
}

// heldWeaponData returns the DataID of the item the local player currently holds (the
// equipment slot whose unique matches itemOnHand), or 0 (e.g. fists).
func (s *session) heldWeaponData() uint32 {
	for _, e := range s.equipment {
		if e.Unique == s.itemOnHand {
			return e.Data
		}
	}
	return 0
}
