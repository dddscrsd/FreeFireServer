package main

import (
	"fmt"
	"log"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// Inventory-slot swap (cmd 119 EQUIP). The client drags an item onto a different loadout slot and sends
// cmd 119 [u8 slot][u32 unique]; the server moves it (swapping with whatever occupied the destination)
// and broadcasts cmd 121 EquipmentChanged for the vacated + destination slots so every client commits the
// change — the swapper's own inventory UI AND remotes' back-mounted weapon models (the slot is what
// RefreshBackMountWeapon reads, see [[cs-full-loadout]]). If the HELD weapon is the one that moved it
// re-asserts the in-hand (cmd 108) so the swap doesn't drop the client to fists. Mirrors the reference
// cmd 119 -> 121 flow; opcodes + wire confirmed for 1.70.1 (Equip=119 JHMFPMMDOLP, EquipmentChanged=121
// LJGFNPIMGMA). We deliberately do NOT re-broadcast a full cmd 174 equip resync here: cmd 121 already
// moves the model, and a full equip re-init would risk dropping the maxed attachments on remotes.
func (s *session) handleEquip(p *packet.Packet) {
	req, ok := message.ParseEquipReq(p.Payload)
	if !ok {
		log.Printf("[mm-udp] cmd=119 EQUIP too short: %x", p.Payload)
		return
	}
	dst, unique := req.Slot, req.Unique

	oldSlot, targetData, equipped := s.equipSlotOf(unique)
	if !equipped {
		it, tracked := s.clientUIDs[unique]
		if !tracked {
			log.Printf("[mm-udp] cmd=119 EQUIP uid=%d not tracked — ignoring", unique)
			return
		}
		targetData, oldSlot = it.data, slotNone
	}
	if oldSlot == dst {
		return // dropped back onto its own slot — nothing to do
	}

	existUnique, existData, occupied := s.equipAt(dst)

	// Update the equipment slot map + the weapon map (which keeps ammo keyed by slot).
	switch {
	case occupied && oldSlot != slotNone: // true swap: destination occupant takes the vacated slot
		s.setEquip(oldSlot, existData, existUnique)
		s.setEquip(dst, targetData, unique)
		s.swapWeaponSlots(oldSlot, dst)
	default: // destination empty (or target was unequipped): fill the destination, vacate the old slot
		if occupied { // an occupied destination with an unequipped target bumps the occupant out of its slot
			s.clearWeaponSlot(dst)
		}
		if oldSlot != slotNone {
			s.clearEquip(oldSlot)
		}
		s.setEquip(dst, targetData, unique)
		s.moveWeaponSlot(oldSlot, dst)
	}

	// cmd 121 to EVERYONE: the vacated slot (target out, occupant in) then the destination slot (target in).
	if oldSlot != slotNone {
		s.match.broadcastData(packet.CmdEquipmentChanged,
			message.EquipmentChanged(s.player.EntityID, oldSlot, unique, existUnique, existData, 0),
			fmt.Sprintf("cmd=121 EquipmentChanged slot=%d out=%d in=%d -> all", oldSlot, unique, existUnique))
	}
	s.match.broadcastData(packet.CmdEquipmentChanged,
		message.EquipmentChanged(s.player.EntityID, dst, 0, unique, targetData, 0),
		fmt.Sprintf("cmd=121 EquipmentChanged slot=%d in=%d data=%d -> all", dst, unique, targetData))

	// Re-assert the in-hand if the HELD weapon is what moved (its unique is unchanged, but the
	// EquipmentChanged can otherwise drop the client back to fists).
	if unique == s.itemOnHand && isActiveWeaponSlot(dst) {
		s.match.broadcastData(packet.CmdChangeHeldItem,
			message.ChangeInventoryOnHand(s.player.EntityID, unique),
			fmt.Sprintf("cmd=108 re-hold uid=%d after slot swap -> all", unique))
	}

	log.Printf("[mm-udp] cmd=119 EQUIP uid=%d data=%d slot %d->%d (occupant uid=%d)", unique, targetData, oldSlot, dst, existUnique)
}

// equipSlotOf returns the loadout slot + DataID currently holding `unique` (found=false if unequipped).
func (s *session) equipSlotOf(unique uint32) (slot byte, data uint32, found bool) {
	for _, e := range s.equipment {
		if e.Unique == unique {
			return e.Slot, e.Data, true
		}
	}
	return slotNone, 0, false
}

// equipAt returns the item (unique+data) currently in `slot`; occupied=false for an empty slot or the
// melee fist placeholder (unique 0/data 0), which is never swapped out.
func (s *session) equipAt(slot byte) (unique, data uint32, occupied bool) {
	for _, e := range s.equipment {
		if e.Slot == slot {
			if e.Unique == 0 && e.Data == 0 {
				return 0, 0, false
			}
			return e.Unique, e.Data, true
		}
	}
	return 0, 0, false
}

// swapWeaponSlots exchanges the weapon-map entries (with their ammo) between two slots, re-keying each to
// its new slot so occupancy checks (buy/pickup override) stay correct after a swap. A slot with no weapon
// (e.g. the grenade/melee slot) is handled by leaving the other side empty.
func (s *session) swapWeaponSlots(a, b byte) {
	wa, oka := s.weapons[a]
	wb, okb := s.weapons[b]
	if okb {
		wb.slot = a
		s.weapons[a] = wb
	} else {
		delete(s.weapons, a)
	}
	if oka {
		wa.slot = b
		s.weapons[b] = wa
	} else {
		delete(s.weapons, b)
	}
}

// moveWeaponSlot moves the weapon-map entry (if any) from `from` to `to` when the destination was empty.
func (s *session) moveWeaponSlot(from, to byte) {
	if from == slotNone {
		return
	}
	if w, ok := s.weapons[from]; ok {
		delete(s.weapons, from)
		w.slot = to
		s.weapons[to] = w
	}
}

// clearWeaponSlot drops the weapon-map entry for a slot whose occupant was bumped out.
func (s *session) clearWeaponSlot(slot byte) { delete(s.weapons, slot) }

// isActiveWeaponSlot reports whether a slot is a drawable weapon slot (primaries / secondary / explosive /
// melee) — the reference re-asserts the in-hand only for these.
func isActiveWeaponSlot(slot byte) bool {
	switch slot {
	case message.SlotPrimary1, message.SlotPrimary2, message.SlotSecondary, message.SlotExplosive, message.SlotMelee:
		return true
	}
	return false
}

// resendEquipTo re-sends this player's full equipment map to a SINGLE target as cmd 121 EquipmentChanged
// (one per occupied slot), so a late joiner (admitLater) or a fresh spectator (emitDeath) renders the
// player's back-mounted / holstered weapons. cmd 121 drives the remote back-mount reliably where a cmd 174
// equip resync doesn't (playerID-gated), and — unlike the additive cmd 174 — re-sending it is idempotent
// (it SETS a slot, so it can't duplicate items for an observer who already has them). See [[cs-full-loadout]].
func (s *session) resendEquipTo(target *session) {
	for _, e := range s.equipment {
		if e.Unique == 0 { // skip the melee fist placeholder (never back-mounted)
			continue
		}
		target.sendDataLog(packet.CmdEquipmentChanged,
			message.EquipmentChanged(s.player.EntityID, e.Slot, 0, e.Unique, e.Data, s.slotRuntime(e.Slot)),
			fmt.Sprintf("cmd=121 EquipmentChanged resync slot=%d uid=%d data=%d ent=%#x -> observer", e.Slot, e.Unique, e.Data, s.entityID))
	}
}

// slotRuntime returns the RuntimeValue to stamp on a slot's cmd 121 EquipmentChanged: an armor piece's
// CURRENT durability (helmet/vest), so the client redraws the armor bar; 0 for every other slot (weapons
// carry no durability in the equipment slot).
func (s *session) slotRuntime(slot byte) uint32 {
	switch slot {
	case message.SlotVest:
		return uint32(s.vestDur)
	case message.SlotHelmet:
		return uint32(s.helmetDur)
	}
	return 0
}

// equipSnapshot captures the current slot->unique map, taken BEFORE a loadout mutation so broadcastEquipDiff
// can tell which slots actually changed.
func (s *session) equipSnapshot() map[byte]uint32 {
	snap := make(map[byte]uint32, len(s.equipment))
	for _, e := range s.equipment {
		snap[e.Slot] = e.Unique
	}
	return snap
}

// broadcastEquipDiff broadcasts a cmd 121 EquipmentChanged to the OTHER players for every loadout slot
// whose item changed vs the `before` snapshot, so remotes keep a player's back-mounted / holstered weapon
// models current after ANY inventory change (buy / pickup / drop / respawn) — not only after a manual slot
// swap. cmd 121 is what reliably drives the remote back-mount: unlike a full cmd 174 equip resync (whose
// apply is gated by a playerID match and, in practice, only takes when the player themselves changes a
// slot), cmd 121 both writes the pawn's per-slot weapon array AND triggers the model refresh. It is sent to
// OTHERS only — the owner already reflects its own change (client prediction + its cmd 174/327). See
// [[cs-full-loadout]].
func (s *session) broadcastEquipDiff(before map[byte]uint32) {
	seen := make(map[byte]bool, len(s.equipment))
	for _, e := range s.equipment { // slots that gained or changed an item
		seen[e.Slot] = true
		if before[e.Slot] != e.Unique {
			s.match.broadcastToOthers(s, packet.CmdEquipmentChanged,
				message.EquipmentChanged(s.player.EntityID, e.Slot, before[e.Slot], e.Unique, e.Data, s.slotRuntime(e.Slot)))
		}
	}
	for slot, oldU := range before { // slots that emptied (present before, absent now)
		if !seen[slot] && oldU != 0 {
			s.match.broadcastToOthers(s, packet.CmdEquipmentChanged,
				message.EquipmentChanged(s.player.EntityID, slot, oldU, 0, 0, 0))
		}
	}
}
