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
		fireState := byte(0)
		if p.firing {
			fireState = 2 // READY: remote clients render the muzzle flash / tracer
		}
		sighting := 0
		if p.sighting {
			sighting = 534 // ADS: remote clients strike the scoped/aiming pose (PRI field 12)
		}
		onHand := uint64(p.itemOnHand)<<32 | uint64(p.heldWeaponData()) // remote pawn's held gun (PRIHPBlock field 4)
		ents = append(ents, message.PRIEntity{RepID: p.repID, Block: message.PRIHPBlock(message.PRIState{
			CurHP: m.entityHP(p.entityID), MaxHP: m.settings.maxHP, Coins: coins, EarnedCoin: uint16(p.award), Score: score,
			VestDur: p.vestDur, HelmetDur: p.helmetDur, ItemOnHand: onHand, Faction: p.faction,
			Kills: capByte(m.kills[p.entityID]), Deaths: capByte(m.deaths[p.entityID]),
			FireState: fireState, Sighting: uint32(sighting), CurEP: byte(p.ep), Damage: m.damage[p.entityID],
		})})
	}
	if m.botEntity != 0 && m.entityHP(m.botEntity) > 0 {
		botScore := message.PackScore(m.teamScore[1], m.teamScore[0]) // the bot is team 2
		ents = append(ents, message.PRIEntity{RepID: botRepID, Block: message.PRIHPBlock(message.PRIState{
			CurHP: m.entityHP(m.botEntity), MaxHP: m.settings.maxHP, Score: botScore, Faction: botFaction,
			Kills: capByte(m.kills[m.botEntity]), Deaths: capByte(m.deaths[m.botEntity]), Damage: m.damage[m.botEntity],
		})})
	}
	return message.SyncPRI(ents)
}

// capByte clamps a match stat count to the u8 PRI scoreboard fields (KILL_COUNT / DEAD_COUNT).
func capByte(v uint16) byte {
	if v > 255 {
		return 255
	}
	return byte(v)
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

	before := s.equipSnapshot() // capture the pre-reset slot map so remotes clear any old back-mounted weapons

	if resetCoins {
		s.coins = csStartingCoins(s.match.settings.baseCoins) // start-of-match wallet = economy table[0]
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
	// NoLoadout custom-room flag (room_setting bit 0x8): skip the free starter USP — the player
	// spawns with fists + medkits only and must BUY every weapon from the shop.
	noLoadout := s.match.settings.noLoadout
	s.equipment = []message.Equipment{
		{Slot: message.SlotMelee, Data: 0, Unique: 0}, // fist (melee slot must always exist)
	}
	s.weapons = map[byte]csWeapon{}
	s.clientUIDs = map[uint32]lootItem{ // medkits always; the USP is added below unless NoLoadout
		medkitID: {unique: medkitID, data: medkitData, count: 2, runtime: 2},
	}
	if !noLoadout {
		s.equipment = append(s.equipment, message.Equipment{Slot: message.SlotSecondary, Data: uspData, Unique: uspUID}) // USP
		s.weapons[message.SlotSecondary] = csWeapon{slot: message.SlotSecondary, data: uspData, unique: uspUID, ammo: pistolAmmo}
		s.clientUIDs[uspUID] = lootItem{unique: uspUID, data: uspData, count: 1, runtime: uspMag}
	}
	if noLoadout {
		s.itemOnHand = 0 // no gun -> hold fists (melee slot)
	} else {
		s.itemOnHand = uspUID
	}
	s.attachments = nil // fresh loadout: forget the old weapons' mounts (the cmd 327 wipe below drops them client-side too)
	s.sendVar(packet.CmdPRISync, s.match.priPayload(), 1)
	s.clearArmor() // the loadout wipe (cmd 327) drops the client's armor too — clear the durability bars
	// Max the starter USP's attachments too (magazine/muzzle) so its ammo is full and it
	// matches bought guns; the attachment items ride the cmd 174 below, then cmd 124 mounts them.
	// medkits ride the cmd 174 always; the USP (+ its ammo reserve + maxed attachments) only when
	// the player gets a free loadout (NoLoadout skips it).
	inv := []message.InvItem{
		{Unique: medkitID, Data: medkitData, Count: 2, Runtime: 2}, // 2x medkits (Runtime = stack count for the HUD)
	}
	var uspAttach []message.InvItem
	var uspEquips []attachEquip
	if !noLoadout {
		uspAttach, uspEquips = s.buildWeaponAttachments(uspUID, uspData)
		inv = append(inv, message.InvItem{Unique: uspUID, Data: uspData, Count: 1, Runtime: uspMag}) // USP weapon
		inv = append(inv, s.giveAmmoStacks(pistolAmmo)...)                                            // pistol-ammo reserve as one combined stack (see giveAmmoStacks)
	}
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
	for _, id := range staleBoxes { // remove stale ground boxes so they vanish on respawn/reset — for EVERY player (the boxes are shared world state)
		s.match.broadcastData(packet.CmdDelContainer, message.DelContainer(id, message.ContainerDynamic),
			fmt.Sprintf("cmd=228 DelContainer box=%d (loadout reset)", id))
	}
	body := message.SyncInventory(s.player.EntityID, inv, uspAttach, equip, onHand)
	s.match.broadcastData(packet.CmdSyncInventory, body, "cmd=174 SyncInventory (USP + ammo) -> all (remotes render the held weapon + skin)")
	s.sendAttachEquips(uspEquips) // mount the starter USP's maxed attachments
	s.broadcastEquipDiff(before)  // tell remotes the slot changes (respawn: drop old back-mounted weapons, show the fresh USP)
}

// currentInventorySync builds a cmd 174 from a player's CURRENT tracked inventory + equipment, so a
// late joiner learns an existing player's items — the held-weapon model (PRI field 4) plus its skin,
// which rides in each item's Runtime (loadedMagFor(data, SkinForWeapon(data, Slots))).
func (s *session) currentInventorySync() []byte {
	inv := make([]message.InvItem, 0, len(s.clientUIDs))
	for _, it := range s.clientUIDs {
		inv = append(inv, message.InvItem{Unique: it.unique, Data: it.data, Count: it.count, Runtime: it.runtime})
	}
	equip := append([]message.Equipment(nil), s.equipment...)
	return message.SyncInventory(s.player.EntityID, inv, nil, equip, s.itemOnHand)
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
	// Refill every ammo reserve the player still holds back to its full total (one combined
	// stack per type now); re-sending the same unique upserts the count.
	for uid, it := range s.clientUIDs {
		if isAmmoData(it.data) {
			inv = append(inv, message.InvItem{Unique: uid, Data: it.data, Count: ammoReserveTotal(it.data)})
		}
	}
	// Refresh medkits to 2 EVERY round (a survivor keeps its loadout, so it never re-gets them
	// via giveLoadout — used kits would stay gone). The cmd 174 handler upserts by unique, so
	// re-sending the tracked medkit unique resets its stack to 2; allocate one if none is tracked.
	medUID := uint32(0)
	for uid, it := range s.clientUIDs {
		if it.data == medkitData {
			medUID = uid
			break
		}
	}
	if medUID == 0 {
		medUID = s.nextUID()
	}
	s.clientUIDs[medUID] = lootItem{unique: medUID, data: medkitData, count: 2, runtime: 2}
	inv = append(inv, message.InvItem{Unique: medUID, Data: medkitData, Count: 2, Runtime: 2})

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
	s.sendMushroomCount() // cmd 533: re-sync the mushroom "N/2" label to this round's count (0 after a round reset)
}

// bindPRIs sends the cmd 118 BindPRI to THIS player, mapping RepIDs to entities so subsequent cmd
// 900 PRI syncs land on the right pawns: its OWN entity FIRST (the client adopts the first
// EntityType=1 binding as its local player), then every other human in the roster, then the enemy
// bot. Re-sent to all humans (bindAll) whenever the roster changes.
func (s *session) bindPRIs() {
	m := s.match
	entries := make([]message.BindEntry, 0, len(m.players)+1)
	entries = append(entries, message.BindEntry{RepID: s.repID, EntityType: message.BindEntityPlayer, EntityGameID: s.entityID})
	for _, p := range m.players {
		if p != s {
			entries = append(entries, message.BindEntry{RepID: p.repID, EntityType: message.BindEntityPlayer, EntityGameID: p.entityID})
		}
	}
	if m.botEntity != 0 { // no bot to bind once it's retired (a 2nd human joined)
		entries = append(entries, message.BindEntry{RepID: botRepID, EntityType: message.BindEntityPlayer, EntityGameID: m.botEntity})
	}
	s.sendDataLog(packet.CmdBindPRI, message.BindPRI(entries),
		fmt.Sprintf("cmd=118 BindPRI self ent=%#x (+%d humans, bot=%#x)", s.entityID, len(m.players)-1, m.botEntity))
}

// bindAll re-sends every human's BindPRI (each self-first) — used when the roster changes on a join
// and on the round-transition rebind, so each client's cmd 900 PRI stream routes the current set.
func (m *Match) bindAll() {
	for _, p := range m.players {
		p.bindPRIs()
	}
}
