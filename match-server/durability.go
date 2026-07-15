package main

import (
	"fmt"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

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

// equipMaxDur is each armor DataID's full durability. The default shop stocks vest 302/303 + helmet
// 305 (see purchase.go armorSlot); a custom room's advanced shop can also stock lvl4 (309/310). lvl4
// values extrapolate the tier progression — the exact number only sets the bar's drain rate (the
// client owns the actual HP reduction), not correctness.
var equipMaxDur = map[uint32]uint16{
	301: 120, 302: 220, 303: 300, 309: 400, // vest lvl1 / lvl2 / lvl3 / lvl4
	304: 120, 305: 220, 306: 300, 310: 400, // helmet lvl1 / lvl2 / lvl3 / lvl4
}

// armorReduction is the fraction of HP damage each piece absorbs — used to back the absorbed amount
// out of the net damage for the durability drop (the client owns the actual HP reduction).
var armorReduction = map[uint32]float64{
	301: 0.25, 302: 0.35, 303: 0.45, 309: 0.55, // vest
	304: 0.30, 305: 0.40, 306: 0.50, 310: 0.55, // helmet
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

// wearArmor drops the hit piece's durability (the client already reduced HP), breaking it at 0, then
// pushes the change so the armor bar redraws (syncArmor).
func (s *session) wearArmor(bodyPart uint8, netDmg uint16) {
	switch bodyPart {
	case bodyHead:
		if s.helmetDur > 0 {
			if s.helmetDur = wearDown(s.helmetDur, netDmg, armorReduction[s.helmetData]); s.helmetDur == 0 {
				s.helmetData = 0 // helmet broke — cleared like the client drops it
			}
			s.syncArmor(message.SlotHelmet)
		}
	case bodyChest:
		if s.vestDur > 0 {
			if s.vestDur = wearDown(s.vestDur, netDmg, armorReduction[s.vestData]); s.vestDur == 0 {
				s.vestData = 0 // vest broke
			}
			s.syncArmor(message.SlotVest)
		}
	}
}

// syncArmor pushes an armor slot's durability change to the client's armor bar. RE (1.70.1): there is NO
// in-place durability message — the equipped vest/helmet's durability is written ONLY when the item OBJECT
// is created (from its Runtime), and neither re-sending the same UID via cmd 174 (touches count only) nor a
// cmd 121 (re-equips the existing item, shows "equipped") moves the bar. So a WEAR (dur > 0) RE-CREATES the
// piece under a FRESH UniqueID carrying the reduced Runtime (cmd 174 → the client news the item at the new
// durability), re-equips it (cmd 121 → the re-equip fires RefreshEquipmentStats, redrawing the bar), then
// drops the stale old item (cmd 327). A BREAK (dur 0) just removes the piece. Owner-targeted: the bar is the
// local HUD and the durability apply is IsLocalPlayer-gated (remotes ignore it).
func (s *session) syncArmor(slot byte) {
	oldUID, data, ok := s.equipAt(slot)
	if !ok {
		return
	}
	dur := s.slotRuntime(slot)
	if dur == 0 { // broke -> remove the piece
		s.clearEquip(slot)
		delete(s.clientUIDs, oldUID)
		s.sendDataLog(packet.CmdEquipmentChanged,
			message.EquipmentChanged(s.player.EntityID, slot, oldUID, 0, 0, 0),
			fmt.Sprintf("cmd=121 armor slot=%d uid=%d broke -> removed", slot, oldUID))
		s.sendDataLog(packet.CmdRemoveInventoryList,
			message.RemoveInventoryList(s.player.EntityID, []uint32{oldUID}),
			fmt.Sprintf("cmd=327 remove broken armor uid=%d", oldUID))
		return
	}
	// wear -> re-create the piece with the reduced durability under a NEW UID (durability is only applied at
	// item creation), re-equip it, then drop the stale old item.
	newUID := s.nextUID()
	delete(s.clientUIDs, oldUID)
	s.trackItem(lootItem{unique: newUID, data: data, count: 1, runtime: dur})
	s.setEquip(slot, data, newUID)
	s.sendDataLog(packet.CmdSyncInventory,
		message.SyncInventory(s.player.EntityID, []message.InvItem{{Unique: newUID, Data: data, Count: 1, Runtime: dur}}, nil, nil, s.itemOnHand),
		fmt.Sprintf("cmd=174 armor refresh slot=%d uid=%d dur=%d (new item)", slot, newUID, dur))
	s.sendDataLog(packet.CmdEquipmentChanged,
		message.EquipmentChanged(s.player.EntityID, slot, oldUID, newUID, data, dur),
		fmt.Sprintf("cmd=121 re-equip armor slot=%d %d->%d dur=%d", slot, oldUID, newUID, dur))
	s.sendDataLog(packet.CmdRemoveInventoryList,
		message.RemoveInventoryList(s.player.EntityID, []uint32{oldUID}),
		fmt.Sprintf("cmd=327 remove stale armor uid=%d", oldUID))
	// The re-create + remove churn above can kick the client out of a held CONSUMABLE's state — a gloo
	// wall in build/placement mode — de-selecting it (armor slots 6/8 don't hit the cmd-121 SwapWeapon
	// path, and the cmd-174 on-hand re-asserts the gloo, so it's the inventory shuffle itself). Re-assert
	// a non-weapon held item so the player keeps holding it. A held WEAPON survives the churn and the
	// client early-returns on an unchanged on-hand, so we only bother for a consumable (gloo/medkit).
	if s.itemOnHand != 0 && !s.isWeaponUID(s.itemOnHand) {
		s.sendDataLog(packet.CmdChangeHeldItem,
			message.ChangeInventoryOnHand(s.player.EntityID, s.itemOnHand),
			fmt.Sprintf("cmd=108 re-assert held consumable uid=%d after armor refresh", s.itemOnHand))
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
