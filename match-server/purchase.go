package main

import (
	"encoding/binary"
	"fmt"
	"log"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// startingCoins is the buy-phase money a player starts a match with (the client also
// defaults its display to 500 until our first cmd 408 result updates it).
const startingCoins = 500

// slotNone marks an item that occupies no loadout slot (ammo, consumables).
const slotNone byte = 0xFF

// weaponAmmo maps a weapon DataID to the ammo DataID it consumes, derived from the
// compound ids in protocol/shop.csv (201=rifle, 202=pistol, 203=sniper, 204=shotgun,
// 205=SMG). Presence in this map is what identifies a DataID as a weapon.
var weaponAmmo = map[uint32]uint32{
	3: 202, 10: 202, 25: 202, 9: 202, // pistols (USP, G18, M500, Desert Eagle)
	8: 205, 7: 205, 32: 205, 15: 205, 88: 205, 43: 205, // SMGs (MP5, UMP, P90, MP40)
	33: 201, 24: 201, 6: 201, 12: 201, // rifles (AN94, Famas, AK, SCAR)
	21: 203, 4: 203, // snipers (Kar98k, AWM)
	5: 204, 41: 204, // shotguns (M1014, M1887)
}

// armorSlot maps an armour DataID to its equipment slot.
var armorSlot = map[uint32]byte{
	302: message.SlotVest, 303: message.SlotVest, // level 2 / level 3 vest
	305: message.SlotHelmet, // level 2 helmet
}

// buildingSlot maps a deployable "building" item (gloo/ice wall) DataID to its OWN loadout
// slot (13). The gloo wall is NOT a grenade: classifying it as a throwable (Filter 4) put
// it in the grenade slot (3), where the client couldn't select it (its gloo-wall button
// maps to slot 13) and it clashed with the weapon/grenade slots. DataIDs 801 (old) / 1201
// (loot) per the reference EQUIP_SLOT_MAP.
var buildingSlot = map[uint32]byte{
	801:  message.SlotBuilding,
	1201: message.SlotBuilding,
}

// csItemPlacement describes how a purchased item enters the loadout.
type csItemPlacement struct {
	slot      byte   // equipment slot, or slotNone; for a primary weapon resolved live
	ammo      uint32 // ammo DataID for a weapon (0 otherwise)
	isWeapon  bool
	isPrimary bool // non-pistol weapon: Primary1/Primary2 chosen against live loadout state
}

// placeItem resolves the item TYPE + fixed slot. A primary weapon's concrete slot
// (Primary1 vs Primary2 vs override) depends on live state, so it is left slotNone here
// and resolved by pickPrimarySlotLocked in addPurchasedItemLocked.
func placeItem(item *message.ShopItem) csItemPlacement {
	if ammo, ok := weaponAmmo[item.ItemID]; ok {
		if ammo == 202 { // pistols equip in the secondary slot
			return csItemPlacement{slot: message.SlotSecondary, ammo: ammo, isWeapon: true}
		}
		return csItemPlacement{slot: slotNone, ammo: ammo, isWeapon: true, isPrimary: true}
	}
	if slot, ok := armorSlot[item.ItemID]; ok {
		return csItemPlacement{slot: slot}
	}
	if slot, ok := buildingSlot[item.ItemID]; ok { // gloo/ice wall -> its own Building slot (not the grenade slot)
		return csItemPlacement{slot: slot}
	}
	if item.Filter == shopFilterThrowable && item.ItemID != itemMushroom {
		return csItemPlacement{slot: message.SlotExplosive} // grenades share the explosive slot
	}
	return csItemPlacement{slot: slotNone} // consumables (repair kit, mushroom): no slot
}

// pickPrimarySlotLocked chooses the slot a newly bought primary weapon fills: an EMPTY
// SlotPrimary1/SlotPrimary2 if either is free; else the slot of the currently HELD primary
// (itemOnHand maps to Primary1/Primary2); else SlotPrimary1. Caller holds invMu.
func (s *session) pickPrimarySlotLocked() byte {
	if _, ok := s.weapons[message.SlotPrimary1]; !ok {
		return message.SlotPrimary1
	}
	if _, ok := s.weapons[message.SlotPrimary2]; !ok {
		return message.SlotPrimary2
	}
	for _, slot := range [...]byte{message.SlotPrimary1, message.SlotPrimary2} {
		if w, ok := s.weapons[slot]; ok && w.unique == s.itemOnHand {
			return slot // override the primary currently in hand
		}
	}
	return message.SlotPrimary1
}

const (
	shopFilterThrowable = 4
	itemMushroom        = 709
)

// ammoStack is the rounds per ammo stack. A weapon's reserve is issued as SEVERAL stacks of
// this size (not one big stack) so dropping ammo drops ONE stack, not the whole reserve.
const ammoStack = 30

// ammoStackCount is how many ammoStack-round stacks a weapon's ammo type is issued.
func ammoStackCount(ammo uint32) int {
	switch ammo {
	case 203, 204: // sniper / shotgun carry fewer
		return 2
	default:
		return 4
	}
}

// isAmmoData reports whether a DataID is an ammo type (201 rifle, 202 pistol, 203 sniper,
// 204 shotgun, 205 SMG) — used to refill ammo stacks between rounds.
func isAmmoData(data uint32) bool { return data >= 201 && data <= 205 }

// giveAmmoStacksLocked issues ammoStackCount(ammo) stacks of ammoStack rounds of the given ammo
// type, each as its own InvItem + unique tracked in clientUIDs, and returns the InvItems. The
// split is what lets a player drop a single stack instead of the whole reserve. Caller holds invMu.
func (s *session) giveAmmoStacksLocked(ammo uint32) []message.InvItem {
	n := ammoStackCount(ammo)
	inv := make([]message.InvItem, 0, n)
	for i := 0; i < n; i++ {
		uid := s.nextUIDLocked()
		inv = append(inv, message.InvItem{Unique: uid, Data: ammo, Count: ammoStack})
		s.trackItemLocked(lootItem{unique: uid, data: ammo, count: ammoStack})
	}
	return inv
}

// handleCSPurchase handles cmd 408: a buy request `[u16 qty][u16 itemId][u16 flag]`.
// It validates the item and price against the shop catalogue, deducts the money, and
// replies with the purchase result (cmd 408, carrying the new balance) followed by an
// inventory sync (cmd 174) that adds the item to the player's loadout.
func (s *session) handleCSPurchase(p *packet.Packet) {
	if len(p.Payload) < 6 {
		log.Printf("[mm-udp] cmd=408 buy req too short (%dB): %x", len(p.Payload), p.Payload)
		return
	}

	qty := uint32(binary.LittleEndian.Uint16(p.Payload[0:]))
	itemID := uint32(binary.LittleEndian.Uint16(p.Payload[2:]))
	if qty == 0 {
		qty = 1
	}
	item, ok := shopItemByID(itemID)
	if !ok {
		log.Printf("[mm-udp] cmd=408 buy of item %d not in catalogue — ignoring", itemID)
		return
	}
	price := item.Price * qty

	s.invMu.Lock()
	if s.coins < price && !cfg.unlimitedMoneyTest {
		coins := s.coins
		s.invMu.Unlock()
		s.sendDataLog(packet.CmdCSPurchase, message.PurchaseResult(purchaseNoMoney, coins),
			fmt.Sprintf("cmd=408 buy DENIED item=%d price=%d coins=%d (insufficient)", itemID, price, coins))
		return
	}

	if !cfg.unlimitedMoneyTest {
		s.coins -= price
	}

	coins := s.coins
	inv, attach, equip, outgoing, attachEquips := s.addPurchasedItemLocked(item, qty)
	onHand := s.itemOnHand
	s.invMu.Unlock()

	// The purchase result drives the "purchase success" popup, but the shop money
	// display and the client's own buy gating read the replicated CUR_COIN, so push
	// the new balance via PRI (cmd 900) immediately — the sync loop also streams it,
	// but an eager unreliable burst updates the UI without the ~300ms wait.
	s.sendDataLog(packet.CmdCSPurchase, message.PurchaseResult(purchaseOK, coins),
		fmt.Sprintf("cmd=408 buy OK item=%d price=%d -> coins=%d", itemID, price, coins))
	s.sendVar(packet.CmdPRISync, s.priPayload(), 2)
	body := message.SyncInventory(s.player.EntityID, inv, attach, equip, onHand)
	s.sendDataLog(packet.CmdSyncInventory, body, fmt.Sprintf("cmd=174 SyncInventory (+item %d x%d, +%d attach)", itemID, qty, len(attach)))
	// Force-equip the maxed attachments onto the bought weapon (cmd 124). The cmd 174 above
	// registered both the weapon and the attachment items in the client's inventory dict, so
	// each cmd 124 can now resolve them by unique.
	s.sendAttachEquips(attachEquips)

	// The new weapon took the slot in the cmd 174 above; the overridden weapon it displaced is
	// dropped to the ground — but only AFTER a short delay (dropOverriddenAfterEquip) so the client
	// has finished equipping the new weapon and the slot is FULL when the drop appears. Otherwise
	// the client's auto-pickup (which only fires into an EMPTY slot) grabs the displaced weapon
	// straight back into the slot the buy just filled. See loot.go for the RE.
	if len(outgoing) > 0 {
		go s.dropOverriddenAfterEquip(outgoing)
	}
}

// purchase result codes (cmd 408 GGKOCCEOFJM): 0 shows the "purchase success" popup
// and fires UI_CURCOIN_CHANGED with the new balance; non-zero shows an error message.
const (
	purchaseOK      byte = 0
	purchaseNoMoney byte = 5
)

// addPurchasedItemLocked appends the bought item (and its ammo, for weapons) to the
// loadout, updates the equipment slot map / in-hand item, and returns the inventory
// DELTA to sync (only the new items — the sync is additive) plus the full equipment
// map. Caller holds invMu.
func (s *session) addPurchasedItemLocked(item *message.ShopItem, qty uint32) (inv, attach []message.InvItem, equip []message.Equipment, outgoing []lootItem, attachEquips []attachEquip) {
	place := placeItem(item)
	if place.isPrimary {
		place.slot = s.pickPrimarySlotLocked()
	}
	uid := s.nextUIDLocked()
	inv = []message.InvItem{{Unique: uid, Data: item.ItemID, Count: qty}}
	if place.isWeapon {
		mag := loadedMagFor(item.ItemID, SkinForWeapon(item.ItemID, s.player.Slots)) // loaded magazine (incl. auto-magazine boost)
		inv[0].Runtime = mag
		s.trackItemLocked(lootItem{unique: uid, data: item.ItemID, count: 1, runtime: mag})
		inv = append(inv, s.giveAmmoStacksLocked(place.ammo)...) // reserve ammo as 30-round stacks
		// Force-equip the best attachment for every slot the weapon supports — added as
		// inventory items here so the follow-up cmd 124 can resolve them by unique (attachment.go).
		attach, attachEquips = s.buildWeaponAttachmentsLocked(uid, item.ItemID)
		// If the resolved slot is already occupied (a 3rd primary overrides the held primary;
		// a 2nd pistol overrides SlotSecondary) capture the outgoing weapon so the caller can
		// drop it to the ground, and untrack its unique (the caller cmd-327s it so the client
		// doesn't hold the same Unique that will sit on the ground). Its ammo is kept.
		if old, occupied := s.weapons[place.slot]; occupied {
			outgoing = append(outgoing, s.weaponLoot(old))
			delete(s.clientUIDs, old.unique)
		}
		s.itemOnHand = uid // switch to the newly bought weapon
		s.weapons[place.slot] = csWeapon{slot: place.slot, data: item.ItemID, unique: uid, ammo: place.ammo}
	} else {
		s.trackItemLocked(lootItem{unique: uid, data: item.ItemID, count: qty})
	}
	if place.slot != slotNone {
		s.setEquipLocked(place.slot, item.ItemID, uid)
	}
	equip = append([]message.Equipment(nil), s.equipment...)
	return
}

// nextUIDLocked allocates a fresh unique item-instance id. Caller holds invMu.
func (s *session) nextUIDLocked() uint32 {
	s.uidCounter++
	return s.uidCounter
}

// setEquipLocked sets (or replaces) the item mapped to a loadout slot. Caller holds invMu.
func (s *session) setEquipLocked(slot byte, data, unique uint32) {
	for i := range s.equipment {
		if s.equipment[i].Slot == slot {
			s.equipment[i] = message.Equipment{Slot: slot, Data: data, Unique: unique}
			return
		}
	}
	s.equipment = append(s.equipment, message.Equipment{Slot: slot, Data: data, Unique: unique})
}
