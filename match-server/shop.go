package main

import "libmadoka/match-server/message"

// csShopTitle is the localization key / title string sent with the shop (cmd 407).
// The client displays whatever string we send; adjust to the real loc key if wanted.
const csShopTitle = "os originais sempre representando."

// csShopItems is the Contra Squad buy-phase catalogue, from protocol/shop.csv. The
// CSV itemid is a compound "<weapon>;<ammo>;<attachments...>"; the shop packet only
// needs the primary item id (the first token) — the bundle is resolved on purchase.
// Filter: 1=weapon, 2=armour/utility, 4=throwable. Limitation = per-round purchase
// cap (0 = unlimited).
//
// TODO (round system): the shop can be re-sent at the start of each buy phase with
// round-specific pricing/availability; for now it's the full static catalogue.
var csShopItems = []message.ShopItem{
	{ItemID: 10, Price: 500, Filter: 1},                  // G18
	{ItemID: 25, Price: 500, Filter: 1},                  // M500
	{ItemID: 9, Price: 900, Filter: 1},                   // Desert Eagle
	{ItemID: 8, Price: 1000, Filter: 1},                  // MP5
	{ItemID: 7, Price: 1100, Filter: 1},                  // UMP
	{ItemID: 21, Price: 1100, Filter: 1},                 // Kar98k
	{ItemID: 33, Price: 1200, Filter: 1},                 // AN94
	{ItemID: 43, Price: 1200, Filter: 1},                 // Thompson
	{ItemID: 24, Price: 1400, Filter: 1},                 // Famas
	{ItemID: 5, Price: 1400, Filter: 1},                  // M1014
	{ItemID: 88, Price: 1500, Filter: 1},                 // MAC-10
	{ItemID: 41, Price: 1600, Filter: 1},                 // M1887
	{ItemID: 6, Price: 2000, Filter: 1},                  // AK
	{ItemID: 12, Price: 2000, Filter: 1},                 // SCAR
	{ItemID: 15, Price: 2100, Filter: 1},                 // MP40
	{ItemID: 4, Price: 2100, Filter: 1},                  // AWM
	{ItemID: 302, Price: 400, Filter: 2},                 // lvl2 vest
	{ItemID: 305, Price: 200, Filter: 2},                 // lvl2 helmet
	{ItemID: 303, Price: 1000, Filter: 2},                // lvl3 vest
	{ItemID: 106, Price: 200, Filter: 2},                 // armour repair kit
	{ItemID: 601, Price: 200, Filter: 4, Limitation: 1},  // frag grenade
	{ItemID: 1201, Price: 300, Filter: 4, Limitation: 3}, // gloo wall (deployable — Building slot 13)
	{ItemID: 709, Price: 100, Filter: 4, Limitation: 2},  // mushroom (max 2/round)
}

// shopItemByID looks up a catalogue entry by its item id (the primary DataID the
// client sends in a cmd 408 buy request).
func shopItemByID(id uint32) (*message.ShopItem, bool) {
	for i := range csShopItems {
		if csShopItems[i].ItemID == id {
			return &csShopItems[i], true
		}
	}
	return nil, false
}
