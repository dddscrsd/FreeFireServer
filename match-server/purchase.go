package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"time"

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

// armorSlot maps an armour DataID to its equipment slot. Covers every CS armour tier so a custom
// room's advanced shop (which can stock lvl4) seeds durability via equipArmor; the default shop only
// stocks lvl2/lvl3.
var armorSlot = map[uint32]byte{
	302: message.SlotVest, 303: message.SlotVest, 309: message.SlotVest, // vest lvl2 / lvl3 / lvl4
	305: message.SlotHelmet, 306: message.SlotHelmet, 310: message.SlotHelmet, // helmet lvl2 / lvl3 / lvl4
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
	isPrimary bool // non-pistol weapon: Primary1/Primary2 chosen against live loadout state~
	isArmor   bool
}

// placeItem resolves the item TYPE + fixed slot. A primary weapon's concrete slot
// (Primary1 vs Primary2 vs override) depends on live state, so it is left slotNone here
// and resolved by pickPrimarySlot in addPurchasedItem.
func placeItem(item *message.ShopItem) csItemPlacement {
	// Advanced custom-room shop items carry a lobby-resolved SlotKind + Ammo (message.ShopItem), so
	// placement needn't hardcode a per-gun map — this is what lets the host stock guns/armour tiers
	// the legacy maps below never knew. Default CS + simple-mode rooms (no SlotKind) fall through to
	// the original map-based logic, so their behaviour is byte-for-byte unchanged.
	if item.SlotKind != "" {
		return placeBySlotKind(item)
	}
	if ammo, ok := weaponAmmo[item.ItemID]; ok {
		if ammo == 202 { // pistols equip in the secondary slot
			return csItemPlacement{slot: message.SlotSecondary, ammo: ammo, isWeapon: true}
		}
		return csItemPlacement{slot: slotNone, ammo: ammo, isWeapon: true, isPrimary: true}
	}
	if slot, ok := armorSlot[item.ItemID]; ok {
		return csItemPlacement{slot: slot, isArmor: true}
	}
	if slot, ok := buildingSlot[item.ItemID]; ok { // gloo/ice wall -> its own Building slot (not the grenade slot)
		return csItemPlacement{slot: slot}
	}
	if item.Filter == shopFilterThrowable && item.ItemID != itemMushroom {
		return csItemPlacement{slot: message.SlotExplosive} // grenades share the explosive slot
	}
	return csItemPlacement{slot: slotNone} // consumables (repair kit, mushroom): no slot
}

// placeBySlotKind resolves placement for an ADVANCED custom-room shop item from its lobby-resolved
// SlotKind (message.ShopItem.SlotKind). The lobby (src/tcp/csadvanced.js) already dropped anything
// the match server can't place, so every reachable item maps to a real slot; the default arm is a
// safety net (treated as a no-slot consumable, never a crash).
func placeBySlotKind(item *message.ShopItem) csItemPlacement {
	switch item.SlotKind {
	case "secondary": // pistol
		return csItemPlacement{slot: message.SlotSecondary, ammo: item.Ammo, isWeapon: true}
	case "primary": // rifle / SMG / sniper / shotgun / LMG — concrete slot chosen live (pickPrimarySlot)
		return csItemPlacement{slot: slotNone, ammo: item.Ammo, isWeapon: true, isPrimary: true}
	case "melee": // sickle etc. — a weapon in the melee slot, no ammo/attachments
		return csItemPlacement{slot: message.SlotMelee, isWeapon: true}
	case "vest":
		return csItemPlacement{slot: message.SlotVest, isArmor: true}
	case "helmet":
		return csItemPlacement{slot: message.SlotHelmet, isArmor: true}
	case "grenade":
		return csItemPlacement{slot: message.SlotExplosive}
	case "building": // gloo wall -> its own Building slot (not the grenade slot)
		return csItemPlacement{slot: message.SlotBuilding}
	case "consumable": // repair kit / adrenaline / mushroom (mushroom takes the instant-EP path in handleCSPurchase)
		return csItemPlacement{slot: slotNone}
	default:
		return csItemPlacement{slot: slotNone}
	}
}

// pickPrimarySlot chooses the slot a newly bought primary weapon fills: an EMPTY
// SlotPrimary1/SlotPrimary2 if either is free; else the slot of the currently HELD primary
// (itemOnHand maps to Primary1/Primary2); else SlotPrimary1.
func (s *session) pickPrimarySlot() byte {
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

// ammoStack is the base per-magazine unit; a weapon's reserve is ammoStackCount of these,
// but issued as ONE combined stack (see ammoReserveTotal / giveAmmoStacks).
const ammoStack = 30

// ammoStackCount is the reserve multiplier for a weapon's ammo type (× ammoStack rounds).
func ammoStackCount(ammo uint32) int {
	switch ammo {
	case 203, 204: // sniper / shotgun carry fewer
		return 2
	default:
		return 4
	}
}

// ammoReserveTotal is a weapon's full spare-ammo reserve for the given ammo type, issued as
// ONE inventory stack (NOT split). The client's reload consume-loop (NPCNMJAGIKI::JBKGGOOPEIN)
// is BUGGY across multiple stacks of the same ammo type: it OVERWRITES the per-stack "consumed"
// out-var instead of accumulating, so a reload spanning more than one stack drains every stack
// but refills the magazine by only the LAST stack's amount ("drains ammo but the clip isn't
// full"). One stack makes that loop run exactly once. Trade-off: dropping ammo now drops the
// whole reserve, not a single 30-round stack.
func ammoReserveTotal(ammo uint32) uint32 { return uint32(ammoStackCount(ammo)) * ammoStack }

// isAmmoData reports whether a DataID is an ammo type (201 rifle, 202 pistol, 203 sniper,
// 204 shotgun, 205 SMG) — used to refill the ammo reserve between rounds.
func isAmmoData(data uint32) bool { return data >= 201 && data <= 205 }

// giveAmmoStacks issues a weapon's full spare-ammo reserve as ONE InvItem + unique tracked in
// clientUIDs, and returns it (a one-element slice so callers can spread it into a cmd 174).
func (s *session) giveAmmoStacks(ammo uint32) []message.InvItem {
	total := ammoReserveTotal(ammo)
	uid := s.nextUID()
	s.trackItem(lootItem{unique: uid, data: ammo, count: total})
	return []message.InvItem{{Unique: uid, Data: ammo, Count: total}}
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
	item, ok := s.match.shopItemByID(itemID)
	if !ok {
		log.Printf("[mm-udp] cmd=408 buy of item %d not in catalogue — ignoring", itemID)
		return
	}
	price := item.Price * qty

	if itemID == itemMushroom && s.mushrooms >= mushroomsPerRound { // round limit -> deny before charging
		s.sendDataLog(packet.CmdCSPurchase, message.PurchaseResult(purchaseNoMoney, s.coins),
			fmt.Sprintf("cmd=408 mushroom DENIED — round limit %d reached", mushroomsPerRound))
		return
	}

	if s.coins < price && !cfg.unlimitedMoneyTest {
		coins := s.coins
		s.sendDataLog(packet.CmdCSPurchase, message.PurchaseResult(purchaseNoMoney, coins),
			fmt.Sprintf("cmd=408 buy DENIED item=%d price=%d coins=%d (insufficient)", itemID, price, coins))
		return
	}

	if !cfg.unlimitedMoneyTest {
		s.coins -= price
	}

	coins := s.coins

	if itemID == itemMushroom { // instant EP grant, not an inventory item (ep.go); round limit pre-checked above
		s.grantMushroom(time.Now())
		s.sendDataLog(packet.CmdCSPurchase, message.PurchaseResult(purchaseOK, coins),
			fmt.Sprintf("cmd=408 buy OK mushroom -> ep=%d (%d/%d this round) coins=%d", s.ep, s.mushrooms, mushroomsPerRound, coins))
		s.sendMushroomCount() // cmd 533: bump the shop cell's "N/2" label (the mushroom has no inventory count to drive it)
		s.sendVar(packet.CmdPRISync, s.match.priPayload(), 2) // eager: push the new EP + coins now
		return
	}

	before := s.equipSnapshot() // capture the slot map so we can tell remotes which slot the buy filled
	inv, attach, equip, outgoing, attachEquips := s.addPurchasedItem(item, qty)
	if slot, isArmor := armorSlot[itemID]; isArmor {
		log.Printf("cmd=408 setting max durability for armor piece, id=%d", itemID)
		s.equipArmor(itemID, slot) // seed durability so the PRI armor bar (fields 2/3) shows full
	}

	onHand := s.itemOnHand

	// The purchase result drives the "purchase success" popup, but the shop money
	// display and the client's own buy gating read the replicated CUR_COIN, so push
	// the new balance via PRI (cmd 900) immediately — the sync loop also streams it,
	// but an eager unreliable burst updates the UI without the ~300ms wait.
	s.sendDataLog(packet.CmdCSPurchase, message.PurchaseResult(purchaseOK, coins),
		fmt.Sprintf("cmd=408 buy OK item=%d price=%d -> coins=%d", itemID, price, coins))
	body := message.SyncInventory(s.player.EntityID, inv, attach, equip, onHand)
	s.match.broadcastData(packet.CmdSyncInventory, body, fmt.Sprintf("cmd=174 SyncInventory (+item %d x%d, +%d attach) -> all", itemID, qty, len(attach)))
	s.sendVar(packet.CmdPRISync, s.match.priPayload(), 1)

	// Force-equip the maxed attachments onto the bought weapon (cmd 124). The cmd 174 above
	// registered both the weapon and the attachment items in the client's inventory dict, so
	// each cmd 124 can now resolve them by unique.
	s.sendAttachEquips(attachEquips)

	// Tell remotes the bought weapon now occupies its slot (cmd 121) so it back-mounts on their view
	// without the buyer having to swap slots. The full cmd 174 above alone doesn't reliably drive the
	// remote back-mount (its apply is playerID-gated); cmd 121 does.
	s.broadcastEquipDiff(before)

	// The new weapon took the slot in the cmd 174 above; the overridden weapon it displaced is
	// dropped to the ground LAST (inline — the sends are reliable and in order), so the client has
	// finished equipping the new weapon and the slot is FULL when the drop appears. Otherwise the
	// client's auto-pickup (which only fires into an EMPTY slot) grabs the displaced weapon straight
	// back into the slot the buy just filled. See loot.go for the RE.
	if len(outgoing) > 0 {
		s.dropOverriddenAfterEquip(outgoing)
	}
	// No extra full-loadout re-sync here: broadcastEquipDiff above already sends the buyer's cmd 121 to
	// the OTHER players (remote pawns → silent, and it lands in their replay recording), which is exactly
	// what a replay needs. Re-sending the buyer its OWN cmd 121 would fire the equip SFX (IsLocalPlayer).
}

// purchase result codes (cmd 408 GGKOCCEOFJM): 0 shows the "purchase success" popup
// and fires UI_CURCOIN_CHANGED with the new balance; non-zero shows an error message.
const (
	purchaseOK      byte = 0
	purchaseNoMoney byte = 5
)

// sendMushroomCount sends cmd 533 (JOOMADHAPPD) with the mushroom's per-round buy count so the shop cell
// renders "N/2". The mushroom is an instant consumable (no inventory item), so the client's m_PurchaseCnt
// — set ONLY by this message — is the sole thing that can move the label off "0/2". Sent after each
// mushroom buy and when the shop opens (which re-syncs it to the round's current count, 0 after a reset).
func (s *session) sendMushroomCount() {
	s.sendDataLog(packet.CmdShopPurchaseCount,
		message.ShopPurchaseCount([]message.PurchaseCount{{ItemID: itemMushroom, Count: uint32(s.mushrooms), Limit: mushroomsPerRound}}),
		fmt.Sprintf("cmd=533 mushroom count %d/%d", s.mushrooms, mushroomsPerRound))
}

// addPurchasedItem appends the bought item (and its ammo, for weapons) to the
// loadout, updates the equipment slot map / in-hand item, and returns the inventory
// DELTA to sync (only the new items — the sync is additive) plus the full equipment
// map.
func (s *session) addPurchasedItem(item *message.ShopItem, qty uint32) (inv, attach []message.InvItem, equip []message.Equipment, outgoing []lootItem, attachEquips []attachEquip) {
	place := placeItem(item)
	if place.isPrimary {
		place.slot = s.pickPrimarySlot()
	}
	uid := s.nextUID()
	var runtime uint32
	if place.isArmor {
		runtime = uint32(equipMaxDur[item.ItemID]) // seed durability so the PRI armor bar (fields 2/3) shows full
	} else {
		runtime = 0
	}
	inv = []message.InvItem{{Unique: uid, Data: item.ItemID, Count: qty, Runtime: runtime}}
	if place.isWeapon {
		mag := loadedMagFor(item.ItemID, SkinForWeapon(item.ItemID, s.player.Slots)) // loaded magazine (incl. auto-magazine boost)
		inv[0].Runtime = mag
		s.trackItem(lootItem{unique: uid, data: item.ItemID, count: 1, runtime: mag})
		if place.ammo != 0 { // melee weapons (sickle) carry no ammo — don't issue a phantom data-0 stack
			inv = append(inv, s.giveAmmoStacks(place.ammo)...) // reserve ammo as 30-round stacks
		}
		// Force-equip the best attachment for every slot the weapon supports — added as
		// inventory items here so the follow-up cmd 124 can resolve them by unique (attachment.go).
		attach, attachEquips = s.buildWeaponAttachments(uid, item.ItemID)
		// If the resolved slot is already occupied (a 3rd primary overrides the held primary;
		// a 2nd pistol overrides SlotSecondary) capture the outgoing weapon so the caller can
		// drop it to the ground, and untrack its unique (the caller cmd-327s it so the client
		// doesn't hold the same Unique that will sit on the ground). Its ammo is kept.
		if old, occupied := s.weapons[place.slot]; occupied {
			outgoing = append(outgoing, s.weaponLoot(old))
			delete(s.clientUIDs, old.unique)
			s.pruneWeaponAttach(old.unique) // the overridden weapon leaves — forget its mounts (late-joiner resync)
		}
		s.itemOnHand = uid // switch to the newly bought weapon
		s.weapons[place.slot] = csWeapon{slot: place.slot, data: item.ItemID, unique: uid, ammo: place.ammo}
	} else if u, existing, dup := s.trackedByData(item.ItemID, 0); dup && !place.isArmor {
		// STACKABLE consumable already held (gloo/medkit/grenade): MERGE the repeat buy into
		// the existing stack — reuse its unique, sum the count — instead of allocating a SECOND
		// data=X item. Duplicate same-data consumables desync count-based use: gloo's glooCount()
		// picks one of the duplicates at random, decrements the wrong stack, hits 0 early, and
		// clears SlotBuilding while walls remain. cmd 174 upserts by Unique (sets an existing
		// unique's count), and a buy is server-authoritative (no client-side predictive item),
		// so reusing the unique stays in sync. Armor is per-instance (durability, not a count) —
		// never merged. The unique allocated above is simply left unused (a harmless id gap).
		existing.count += qty
		s.clientUIDs[u] = existing
		uid = u
		inv = []message.InvItem{{Unique: u, Data: item.ItemID, Count: existing.count, Runtime: existing.runtime}}
	} else {
		s.trackItem(lootItem{unique: uid, data: item.ItemID, count: qty, runtime: runtime})
	}
	if place.slot != slotNone {
		s.setEquip(place.slot, item.ItemID, uid)
	}
	equip = append([]message.Equipment(nil), s.equipment...)
	return
}

// nextUID allocates a fresh unique item-instance id.
func (s *session) nextUID() uint32 {
	s.uidCounter++
	return s.uidCounter
}

// setEquip sets (or replaces) the item mapped to a loadout slot.
func (s *session) setEquip(slot byte, data, unique uint32) {
	for i := range s.equipment {
		if s.equipment[i].Slot == slot {
			s.equipment[i] = message.Equipment{Slot: slot, Data: data, Unique: unique}
			return
		}
	}
	s.equipment = append(s.equipment, message.Equipment{Slot: slot, Data: data, Unique: unique})
}
