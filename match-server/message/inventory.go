package message

// Player inventory sync (cmd 174, message BGKCMKNDAGA) and shop purchase result
// (cmd 408, message IIOCLGDNBBB). Reverse-engineered from FF 1.70.1 — see
// [[cs-shop-inventory]]. WriteString/count details verified against the client's
// Serialize methods.

// Inventory item slots (from the reference dump). A melee entry (slot Melee, data 0,
// unique 0) must always be present in the equipment list.
const (
	SlotPrimary1  byte = 0
	SlotPrimary2  byte = 1
	SlotSecondary byte = 2 // pistol (e.g. USP)
	SlotExplosive byte = 3 // grenade
	SlotMelee     byte = 4 // fist / melee weapon
	SlotHelmet    byte = 6
	SlotBag       byte = 7
	SlotVest      byte = 8
)

// InvItem is one inventory or attachment entry (JPALKHEHFIM): a unique runtime
// instance id, the item DataID (e.g. 3 = USP), a stack count, and a runtime state.
type InvItem struct {
	Unique  uint32
	Data    uint32
	Count   uint32
	Runtime uint32
}

// Equipment binds a loadout Slot to an item's DataID + Unique (JOLBCNEHKLJ).
type Equipment struct {
	Slot   byte
	Data   uint32
	Unique uint32
}

// SyncInventory builds the cmd 174 payload (BGKCMKNDAGA): the player's inventory,
// attachments, and slot→item equipment mapping, plus the in-hand item. Verified from
// BGKCMKNDAGA::Serialize @0x37dc960 — the list counts are i16 (NOT u32 like the older
// reference). The client's handler ADDS these to its item pool, so this must be sent
// exactly ONCE (reliably) per state change or items/ammo duplicate.
func SyncInventory(playerID uint32, inventory, attachments []InvItem, equipment []Equipment, itemOnHand uint32) []byte {
	w := &Writer{}
	w.U32(playerID)
	writeInvList(w, inventory)
	writeInvList(w, attachments)
	w.I16(int16(len(equipment)))
	for _, e := range equipment {
		w.U8(e.Slot)
		w.U32(e.Data)
		w.U32(e.Unique)
	}
	w.U32(itemOnHand)
	w.I32(0) // PHLFCPAOBMJ (unknown; 0)
	w.I32(0) // JGHFAGHIGOG (unknown; 0)
	return w.B
}

func writeInvList(w *Writer, items []InvItem) {
	w.I16(int16(len(items)))
	for _, it := range items {
		w.U32(it.Unique)
		w.U32(it.Data)
		w.U32(it.Count)
		w.U32(it.Runtime)
	}
}

// PurchaseResult builds the cmd 408 result payload (IIOCLGDNBBB, server->client). The
// client reads result (0 = success -> "purchase success" popup) and coins (the new
// balance, dispatched as UI_CURCOIN_CHANGED to update the shop money display). The
// item list is ignored by the client's result handler, so it is sent empty. Verified
// from IIOCLGDNBBB::Serialize @0x369a558.
func PurchaseResult(result byte, coins uint32) []byte {
	w := &Writer{}
	w.U8(result)
	w.U32(coins)
	w.I16(0) // JFNPIIHICEH list (empty)
	return w.B
}
