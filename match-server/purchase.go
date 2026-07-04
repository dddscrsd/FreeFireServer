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
	8: 205, 7: 205, 32: 205, 15: 205, // SMGs (MP5, UMP, P90, MP40)
	33: 201, 24: 201, 6: 201, 12: 201, // rifles (AN94, Famas, AK, SCAR)
	21: 203, 4: 203, // snipers (Kar98k, AWM)
	5: 204, 41: 204, // shotguns (M1014, M1887)
}

// armorSlot maps an armour DataID to its equipment slot.
var armorSlot = map[uint32]byte{
	302: message.SlotVest, 303: message.SlotVest, // level 2 / level 3 vest
	305: message.SlotHelmet, // level 2 helmet
}

// csItemPlacement describes how a purchased item enters the loadout.
type csItemPlacement struct {
	slot     byte   // equipment slot, or slotNone
	ammo     uint32 // ammo DataID for a weapon (0 otherwise)
	isWeapon bool
}

// placeItem resolves where a shop item goes in the loadout.
func placeItem(item *message.ShopItem) csItemPlacement {
	if ammo, ok := weaponAmmo[item.ItemID]; ok {
		slot := message.SlotPrimary1
		if ammo == 202 { // pistols equip in the secondary slot
			slot = message.SlotSecondary
		}
		return csItemPlacement{slot: slot, ammo: ammo, isWeapon: true}
	}
	if slot, ok := armorSlot[item.ItemID]; ok {
		return csItemPlacement{slot: slot}
	}
	if item.Filter == shopFilterThrowable && item.ItemID != itemMushroom {
		return csItemPlacement{slot: message.SlotExplosive} // grenades share the explosive slot
	}
	return csItemPlacement{slot: slotNone} // consumables (repair kit, mushroom): no slot
}

const (
	shopFilterThrowable = 4
	itemMushroom        = 709
)

// ammoCountFor returns a sensible reserve for a freshly bought weapon's ammo type.
func ammoCountFor(ammo uint32) uint32 {
	switch ammo {
	case 203, 204: // sniper / shotgun carry fewer rounds
		return 48
	default:
		return 120
	}
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
	if s.coins < price {
		coins := s.coins
		s.invMu.Unlock()
		s.sendDataLog(packet.CmdCSPurchase, message.PurchaseResult(purchaseNoMoney, coins),
			fmt.Sprintf("cmd=408 buy DENIED item=%d price=%d coins=%d (insufficient)", itemID, price, coins))
		return
	}
	s.coins -= price
	coins := s.coins
	inv, equip := s.addPurchasedItemLocked(item, qty)
	onHand := s.itemOnHand
	s.invMu.Unlock()

	// The purchase result drives the "purchase success" popup, but the shop money
	// display and the client's own buy gating read the replicated CUR_COIN, so push
	// the new balance via PRI (cmd 900) immediately — the sync loop also streams it,
	// but an eager unreliable burst updates the UI without the ~300ms wait.
	s.sendDataLog(packet.CmdCSPurchase, message.PurchaseResult(purchaseOK, coins),
		fmt.Sprintf("cmd=408 buy OK item=%d price=%d -> coins=%d", itemID, price, coins))
	s.sendVar(packet.CmdPRISync, s.priPayload(), 2)
	body := message.SyncInventory(s.player.EntityID, inv, nil, equip, onHand)
	s.sendDataLog(packet.CmdSyncInventory, body,
		fmt.Sprintf("cmd=174 SyncInventory (+item %d x%d)", itemID, qty))
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
func (s *session) addPurchasedItemLocked(item *message.ShopItem, qty uint32) ([]message.InvItem, []message.Equipment) {
	place := placeItem(item)
	uid := s.nextUIDLocked()
	inv := []message.InvItem{{Unique: uid, Data: item.ItemID, Count: qty}}
	if place.isWeapon {
		ammoUID := s.nextUIDLocked()
		inv = append(inv, message.InvItem{Unique: ammoUID, Data: place.ammo, Count: ammoCountFor(place.ammo)})
		s.itemOnHand = uid // switch to the newly bought weapon
	}
	if place.slot != slotNone {
		s.setEquipLocked(place.slot, item.ItemID, uid)
	}
	return inv, append([]message.Equipment(nil), s.equipment...)
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
