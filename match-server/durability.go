package main

import "libmadoka/match-server/message"

// Armor durability (helmet + vest). The CLIENT applies the armor damage-reduction to its own HP; the
// server only TRACKS + streams each piece's durability (PRI fields 2/3) so armor bars show, deplete,
// and break — on the local player and on remotes. A body hit wears the vest, a head hit the helmet,
// by the absorbed damage backed out of the client's net (post-armor) damage and the piece's reduction.
// Set to full on purchase; a piece that reaches 0 breaks and is cleared (matching the client dropping
// broken armor). Durability persists across rounds for survivors; giveLoadout clears it for a player
// who died (its inventory wipe drops the armor too).

// hit body parts (EColliderType): 1=HEAD, 2=BODY, 3=LIMB (see combat.go handleTakeDamage).
const (
	bodyHead  uint8 = 1
	bodyChest uint8 = 2
)

// equipMaxDur is each armor DataID's full durability. Our shop stocks vest 302/303 + helmet 305 (see
// purchase.go armorSlot); values from the reference EQUIP_MAX_DURABILITY.
var equipMaxDur = map[uint32]uint16{
	301: 120, 302: 220, 303: 300, // vest lvl1 / lvl2 / lvl3
	304: 120, 305: 220, 306: 300, // helmet lvl1 / lvl2 / lvl3
}

// armorReduction is the fraction of HP damage each piece absorbs — used to back the absorbed amount
// out of the net damage for the durability drop (the client owns the actual HP reduction).
var armorReduction = map[uint32]float64{
	301: 0.25, 302: 0.35, 303: 0.45, // vest
	304: 0.30, 305: 0.40, 306: 0.50, // helmet
}

// equipArmor records a bought vest/helmet + seeds its durability to full.
func (s *session) equipArmor(dataID uint32, slot byte) {
	switch slot {
	case message.SlotVest:
		s.vestData, s.vestDur = dataID, equipMaxDur[dataID]
	case message.SlotHelmet:
		s.helmetData, s.helmetDur = dataID, equipMaxDur[dataID]
	}
}

// wearArmor drops the hit piece's durability (the client already reduced HP), breaking it at 0.
func (s *session) wearArmor(bodyPart uint8, netDmg uint16) {
	switch bodyPart {
	case bodyHead:
		if s.helmetDur > 0 {
			if s.helmetDur = wearDown(s.helmetDur, netDmg, armorReduction[s.helmetData]); s.helmetDur == 0 {
				s.helmetData = 0 // helmet broke — cleared like the client drops it
			}
		}
	case bodyChest:
		if s.vestDur > 0 {
			if s.vestDur = wearDown(s.vestDur, netDmg, armorReduction[s.vestData]); s.vestDur == 0 {
				s.vestData = 0 // vest broke
			}
		}
	}
}

// wearDown returns dur reduced by the absorbed damage = net * r/(1-r), clamped at 0 (>=1 per hit so
// an armored hit always shows wear). r is the piece's reduction; a missing entry falls back to 0.3.
func wearDown(dur, netDmg uint16, r float64) uint16 {
	if r <= 0 || r >= 1 {
		r = 0.3
	}
	absorbed := uint16(float64(netDmg) * r / (1 - r))
	if absorbed < 1 {
		absorbed = 1
	}
	if absorbed >= dur {
		return 0
	}
	return dur - absorbed
}

// clearArmor drops both pieces (used when a died player's loadout is wiped — the client drops the
// armor with the rest of the inventory, so the durability bars must clear too).
func (s *session) clearArmor() {
	s.vestData, s.vestDur = 0, 0
	s.helmetData, s.helmetDur = 0, 0
}
