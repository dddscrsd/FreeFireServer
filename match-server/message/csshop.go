package message

// Contra Squad shop (LOJA) — cmd 407, typed message NDBLNLLMMJA. The handler
// KPDMJKOEHEE::DPJJPAAFANL @0x196a140 routes it to the shop controller LPGDKKAGPKJ
// (requires the match to be Running). Wire format verified from NDBLNLLMMJA::Serialize
// @0x34940c4 + BENHAIGLOGI::Serialize @0x37db794 + WriteString @0x33021b4:
//
//	count : i16 (LE)                            -- number of items
//	repeat count times (BENHAIGLOGI, 11 bytes):
//	  ItemID     : u32
//	  Price      : u32
//	  Filter     : u8   -- category (1=weapon, 2=armour/utility, 4=throwable)
//	  Limitation : u8   -- per-round purchase cap (0 = unlimited)
//	  Bonus      : u8   -- bool; weapon kill-reward flag
//	locKey : string (i32 UTF-8 byte count + bytes)  -- shop title / localization key
//	flag   : u8 (bool)                              -- unknown (probably a display flag)

// ShopItem is one CS shop entry (BENHAIGLOGI). ItemID/Price/Filter/Limitation/Bonus are the wire
// fields serialized into cmd 407 (CSShop). SlotKind + Ammo are NOT sent to the client — they carry
// the loadout-placement metadata a CUSTOM-ROOM advanced shop resolves lobby-side (see the settings
// pipeline / purchase.placeItem); they are empty/0 for the default package catalogue, which keeps
// using the legacy hardcoded placement maps.
type ShopItem struct {
	ItemID     uint32
	Price      uint32
	Filter     byte
	Limitation byte
	Bonus      bool

	// SlotKind (advanced custom-room items only): "primary" | "secondary" | "melee" | "vest" |
	// "helmet" | "grenade" | "building" | "consumable". Empty => placeItem uses the legacy maps.
	SlotKind string
	// Ammo is the weapon's ammo DataID (201 rifle / 202 pistol / 203 sniper / 204 shotgun / 205 SMG),
	// resolved lobby-side from the shop CSV so placement needn't hardcode a per-gun ammo map.
	Ammo uint32
}

// PurchaseCount is one limited item's per-round buy count (CHEEGFNLIOE): the count the player has bought
// this round + the round limit.
type PurchaseCount struct {
	ItemID uint32
	Count  uint32
	Limit  uint32
}

// ShopPurchaseCount builds the cmd 533 payload (server->client, message JOOMADHAPPD): the per-round
// PURCHASE COUNT for limited shop items. The client's OnPurchaseSuccess (event UI_CSPURCHASE_SUCCESS)
// reads it into the shop cell's m_PurchaseCnt, which drives the "N/limit" label — the cmd 408 result does
// NOT carry this (its item list is ignored), so an instant consumable like the mushroom (no inventory
// count) shows "0/limit" forever without it. Wire (LE): [i16 count] then per entry {u32 ItemID, u32 Count,
// u32 Limit}; Limit 0 falls back to the cmd 407 shop Limitation. Verified from JOOMADHAPPD::Serialize.
func ShopPurchaseCount(entries []PurchaseCount) []byte {
	w := &Writer{}
	w.I16(int16(len(entries)))
	for _, e := range entries {
		w.U32(e.ItemID)
		w.U32(e.Count)
		w.U32(e.Limit)
	}
	return w.B
}

// CSShop builds the cmd 407 payload: the item list, a localization/title string, and
// a trailing flag.
func CSShop(items []ShopItem, locKey string, flag bool) []byte {
	w := &Writer{}
	w.I16(int16(len(items))) // count is i16 (2 bytes), not u32
	for _, it := range items {
		w.U32(it.ItemID)
		w.U32(it.Price)
		w.U8(it.Filter)
		w.U8(it.Limitation)
		w.Bool(it.Bonus)
	}
	w.Str(locKey)
	w.Bool(flag)
	return w.B
}
