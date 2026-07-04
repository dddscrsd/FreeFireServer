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

// ShopItem is one CS shop entry (BENHAIGLOGI).
type ShopItem struct {
	ItemID     uint32
	Price      uint32
	Filter     byte
	Limitation byte
	Bonus      bool
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
