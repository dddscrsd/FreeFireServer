package main

import (
	"fmt"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// maxRound is the CS match length (GRI MaxRound); roundsToWin ends the match. Only the
// local team scores against the passive bot, so the match ends after roundsToWin wins.
const (
	maxRound    = 7
	roundsToWin = (maxRound + 1) / 2 // 4
)

// priPayload builds the PRI (cmd 900) block stream for the WHOLE match: one block per human in
// the roster (HP + live coins/award + round score from that player's own team perspective +
// faction) plus the current-round bot (HP + opposite faction) while it is alive. A dead bot is
// dropped so its death (cmd 107) is not undone by a fresh HP update. Broadcast identically to
// every human — each client reads its OWN entity's block by RepID, so one encode serves all.
func (m *Match) priPayload() []byte {
	ents := make([]message.PRIEntity, 0, len(m.players)+1)
	for _, p := range m.players {
		coins := uint16(p.coins)
		if cfg.unlimitedMoneyTest {
			coins = 9999
		}
		// score from THIS player's team perspective (my wins, opp wins) so the client resolves
		// my/oppo consistently whoever's score changed.
		score := message.PackScore(m.teamScore[p.team-1], m.teamScore[2-p.team])
		ents = append(ents, message.PRIEntity{RepID: p.repID, Block: message.PRIHPBlock(m.entityHP(p.entityID), maxHP, coins, uint16(p.award), score, p.faction)})
	}
	if m.entityHP(m.botEntity) > 0 {
		botScore := message.PackScore(m.teamScore[1], m.teamScore[0]) // the bot is team 2
		ents = append(ents, message.PRIEntity{RepID: botRepID, Block: message.PRIHPBlock(m.entityHP(m.botEntity), maxHP, 0, 0, botScore, botFaction)})
	}
	return message.SyncPRI(ents)
}

func (s *session) resyncWallsPri() []byte {
	// Snapshot each live gloo wall's (RepID, state) so its per-wall PRI rides the same
	// cmd 900 stream as the player/bot.
	type wallSnap struct {
		rep   uint32
		state byte
	}
	walls := make([]wallSnap, 0, len(s.match.walls))
	for _, w := range s.match.walls {
		walls = append(walls, wallSnap{rep: w.id, state: w.state})
	}
	ents := []message.PRIEntity{}
	for _, w := range walls { // one per-wall state block, keyed by the wall's own RepID
		ents = append(ents, message.PRIEntity{RepID: w.rep, Block: iceWallPRIBlock(w.state)})
	}

	return message.SyncPRI(ents)
}

// sendStartingLoadout gives the player their initial inventory at match start (seeding
// the economy). See giveLoadout.
func (s *session) sendStartingLoadout() { s.giveLoadout(true) }

// giveLoadout resets the player to the base loadout. It FIRST clears whatever the client
// currently holds — cmd 327 (RemoveInventoryList) over every tracked unique — then issues a
// fresh USP (data 3) in the pistol slot + pistol ammo (data 202) via cmd 174. The clear is
// essential: a pending-revive death keeps the pawn, so the client never drops its loadout on
// its own, and cmd 174 can only ADD, so without the wipe the player would keep every stale
// weapon/gloo-wall/armour across a respawn. This makes the respawn start fresh with ONLY the
// USP. At match start (resetCoins) it also seeds the economy; on a revive (resetCoins=false)
// it keeps the accumulated coins so the player re-buys with their kept money.
func (s *session) giveLoadout(resetCoins bool) {
	const uspData = 3
	const pistolAmmo = 202

	if resetCoins {
		s.coins = startingCoins
	}
	// Snapshot the client's current items so cmd 327 can DELETE them before we re-add. Do
	// NOT reset uidCounter (keep it monotonic) so the fresh USP's unique can't collide with
	// a stale item that hasn't been cleared yet.
	stale := make([]uint32, 0, len(s.clientUIDs))
	for uid := range s.clientUIDs {
		stale = append(stale, uid)
	}
	// Snapshot + clear any ground-loot boxes so a respawn/round-reset wipes stale containers
	// (the round world resets); each id gets a cmd 228 DelContainer below.
	var staleBoxes []uint16
	for id := range s.match.containers {
		staleBoxes = append(staleBoxes, id)
	}
	s.match.containers = map[uint16]*container{}
	uspUID := s.nextUID()
	medkitID := s.nextUID()
	uspMag := loadedMagFor(uspData, SkinForWeapon(uspData, s.player.Slots)) // loaded magazine (incl. auto-magazine boost)
	s.equipment = []message.Equipment{
		{Slot: message.SlotMelee, Data: 0, Unique: 0},                // fist (melee slot must always exist)
		{Slot: message.SlotSecondary, Data: uspData, Unique: uspUID}, // USP
	}
	s.weapons = map[byte]csWeapon{
		message.SlotSecondary: {slot: message.SlotSecondary, data: uspData, unique: uspUID, ammo: pistolAmmo},
	}
	s.clientUIDs = map[uint32]lootItem{ // seed with the USP + medkits; giveAmmoStacks adds the ammo stacks
		uspUID:   {unique: uspUID, data: uspData, count: 1, runtime: uspMag},
		medkitID: {unique: medkitID, data: medkitData, count: 2, runtime: 2},
	}
	s.itemOnHand = uspUID
	// Max the starter USP's attachments too (magazine/muzzle) so its ammo is full and it
	// matches bought guns; the attachment items ride the cmd 174 below, then cmd 124 mounts them.
	uspAttach, uspEquips := s.buildWeaponAttachments(uspUID, uspData)
	inv := append([]message.InvItem{
		{Unique: uspUID, Data: uspData, Count: 1, Runtime: uspMag}, // USP weapon
		{Unique: medkitID, Data: medkitData, Count: 2, Runtime: 2}, // 2x medkits (Runtime = stack count for the HUD)
	}, s.giveAmmoStacks(pistolAmmo)...) // pistol-ammo reserve as 30-round stacks
	if cfg.infiniteGloo { // testing: seed gloo walls so placing needs no shop trip
		glooUID := s.nextUID()
		s.equipment = append(s.equipment, message.Equipment{Slot: message.SlotBuilding, Data: glooDataID, Unique: glooUID})
		s.clientUIDs[glooUID] = lootItem{unique: glooUID, data: glooDataID, count: 5}
		inv = append(inv, message.InvItem{Unique: glooUID, Data: glooDataID, Count: 5})
	}
	equip := append([]message.Equipment(nil), s.equipment...)
	onHand := s.itemOnHand

	if len(stale) > 0 { // wipe the previous loadout FIRST so the respawn starts fresh (USP only)
		s.sendDataLog(packet.CmdRemoveInventoryList, message.RemoveInventoryList(s.player.EntityID, stale),
			fmt.Sprintf("cmd=327 RemoveInventoryList (clear %d stale items)", len(stale)))
	}
	for _, id := range staleBoxes { // remove stale ground boxes so they vanish on respawn/reset
		s.sendDataLog(packet.CmdDelContainer, message.DelContainer(id, message.ContainerDynamic),
			fmt.Sprintf("cmd=228 DelContainer box=%d (loadout reset)", id))
	}
	body := message.SyncInventory(s.player.EntityID, inv, uspAttach, equip, onHand)
	s.sendDataLog(packet.CmdSyncInventory, body, "cmd=174 SyncInventory (USP + ammo)")
	s.sendAttachEquips(uspEquips) // mount the starter USP's maxed attachments
}

// reissueLoadout refills every weapon currently in the loadout to a full magazine (and
// tops up reserve ammo) at the start of a new round for a SURVIVING player, WITHOUT
// allocating new item instances: the cmd 174 handler upserts inventory by Unique
// (SyncInventoryInfo -> OJKKGKBGKMJ sets an existing weapon's clip = InvItem.Runtime via
// KANLCBHFONB::NKJJELOAGGL), so re-sending the SAME uniques resets ammo, not duplicates.
func (s *session) reissueLoadout() {
	inv := make([]message.InvItem, 0, len(s.weapons)+len(s.clientUIDs))
	for _, w := range s.weapons {
		inv = append(inv, message.InvItem{
			Unique:  w.unique,
			Data:    w.data,
			Count:   1,
			Runtime: loadedMagFor(w.data, SkinForWeapon(w.data, s.player.Slots)), // full magazine (incl. auto-magazine boost)
		})
	}
	// Refill every ammo stack the player still holds back to a full 30 (ammo is decoupled from
	// weapons now the reserve is split into stacks); re-sending the same unique upserts the count.
	for uid, it := range s.clientUIDs {
		if isAmmoData(it.data) {
			inv = append(inv, message.InvItem{Unique: uid, Data: it.data, Count: ammoStack})
		}
	}
	equip := append([]message.Equipment(nil), s.equipment...)
	onHand := s.itemOnHand
	if len(inv) == 0 {
		return
	}
	body := message.SyncInventory(s.player.EntityID, inv, nil, equip, onHand)
	s.sendDataLog(packet.CmdSyncInventory, body, "cmd=174 SyncInventory (round restart — refill mags/ammo)")
}

// sendCSShop sends the Contra Squad shop item list (cmd 407). The client's LOJA UI
// reads this list when it opens during the buy phase.
func (s *session) sendCSShop() {
	s.sendDataLog(packet.CmdCSShop, message.CSShop(csShopItems, csShopTitle, false),
		fmt.Sprintf("cmd=407 CSShop (%d items, %q)", len(csShopItems), csShopTitle))
}

// bindPRIs sends the cmd 118 BindPRI that maps RepIDs to entities so subsequent cmd 900
// PRI syncs land on them: the local player FIRST (the client adopts the first
// EntityType=1 binding as its own), then the current-round enemy bot.
func (s *session) bindPRIs() {
	bind := message.BindPRI([]message.BindEntry{
		{RepID: playerRepID, EntityType: message.BindEntityPlayer, EntityGameID: playerEntityID},
		{RepID: botRepID, EntityType: message.BindEntityPlayer, EntityGameID: s.match.botEntity},
	})
	s.sendDataLog(packet.CmdBindPRI, bind,
		fmt.Sprintf("cmd=118 BindPRI local ent=%#x + bot ent=%#x", uint32(playerEntityID), s.match.botEntity))
}
