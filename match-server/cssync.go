package main

import (
	"fmt"
	"log"
	"time"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// maxRound is the CS match length (GRI MaxRound); roundsToWin ends the match. Only the
// local team scores against the passive bot, so the match ends after roundsToWin wins.
const (
	maxRound    = 7
	roundsToWin = (maxRound + 1) / 2 // 4
)

// startCSSync launches the CS match-state streaming loop once per session. Safe to
// call from both the fresh-join and the mid-match reconnect paths.
func (s *session) startCSSync() {
	if s.syncStarted {
		return
	}
	s.syncStarted = true
	log.Printf("[mm-udp] -> starting CS sync loop (GRI %d + PRI %d via VAR/so=4, BindPRI %d) %v",
		packet.CmdGRISync, packet.CmdPRISync, packet.CmdBindPRI, s.remote)
	go s.csSyncLoop()
}

// csSyncLoop streams the Contra Squad match state and drives the round loop: after a
// shop warmup, each round runs a Prepare (buy) phase then a Fight phase that ends when
// the enemy bot dies; the local team then scores and the next round re-picks the arena,
// teleports the player, and respawns a fresh bot near it. Bounded per session.
func (s *session) csSyncLoop() {
	s.invMu.Lock()
	s.coins = startingCoins
	s.invMu.Unlock()
	s.matchStart = time.Now() // origin for the CS phase-countdown clock
	go s.serverTimeLoop()     // sync the client's match clock so the CS timers count down
	if s.round == 0 {         // reconnect path skips the join handler that seeds round 1
		s.round = 1
		s.botEntity = botEntityID
	}
	s.initHP()

	time.Sleep(150 * time.Millisecond) // BindPRI must arrive after cmd 101 created the entities
	s.bindPRIs()
	s.shopWarmup()

	for s.round <= maxRound && s.teamScore[0] < roundsToWin {
		// No teleport pull in Prepare: a dead player was revived + repositioned at the new
		// gate via cmd 388, an alive player was teleported during the black window, so the
		// buy phase is free-roam.
		if !s.streamPhase(message.CSPhasePrepare, cfg.buyPhase, nil, false) {
			return
		}
		if cfg.holdPrepare { // dev shop-testing mode: never leave Prepare
			s.streamPhase(message.CSPhasePrepare, 6*time.Hour, nil, false)
			return
		}
		// Fight: run the shrinking SafeZone; the round ends when the bot OR the local
		// player dies (out-of-zone damage can eliminate the player).
		zoneStop := make(chan struct{})
		go s.runSafeZone(zoneStop)
		ok := s.streamPhase(message.CSPhaseFight, 6*time.Hour, nil, true)
		close(zoneStop)
		if !ok {
			return
		}
		// Decide the round: local team wins if the player survived (bot died), else the
		// enemy team wins (the player was eliminated).
		localWon := s.entityHP(playerEntityID) > 0
		winnerTeam := byte(localTeamID)
		if localWon {
			s.teamScore[0]++
		} else {
			winnerTeam = enemyTeamID
			s.teamScore[1]++
		}
		// Award round coins: 500 per kill by the local player + 500 for winning the round.
		// s.coins streams as PRI field 32, so the shop shows the new balance next buy phase.
		award := 500 * s.roundKills.Swap(0)
		award += 500 * uint32(s.round)
		if localWon {
			award += 500 // win bonus
		}

		s.invMu.Lock()
		s.coins += award
		s.award = award
		coins := s.coins
		s.invMu.Unlock()
		log.Printf("[mm-udp] ROUND %d won by team %d (score %d-%d) +%d coins (=%d) %v",
			s.round, winnerTeam, s.teamScore[0], s.teamScore[1], award, coins, s.remote)

		// Match over when a team reaches the win target: skip the round-result, pause on
		// the final kill, then send MatchEnd (cmd 103) which shows the result and leaves.
		if s.teamScore[0] >= roundsToWin || s.teamScore[1] >= roundsToWin || s.round >= maxRound {
			time.Sleep(500 * time.Millisecond)
			s.matchEnd(localWon)
			break
		}
		s.roundResult(winnerTeam) // cmd 409 round-result (non-deciding rounds only)
		// Dead player -> revive via cmd 388; alive/won player -> teleport in the black window.
		s.roundTransition(!localWon) // Post banner + fade-to-black -> reposition -> Introduction (bumps s.round)
	}
	log.Printf("[mm-udp] match over — local team wins %d-%d %v", s.teamScore[0], s.teamScore[1], s.remote)
	s.streamPhase(message.CSPhasePost, 6*time.Hour, nil, false)
}

// streamPhase streams the GRI (round + phase) and the local+bot PRI at ~3/s for up to
// dur. If pull is non-nil it also streams a force-teleport (cmd 145) pinning the local
// player there — used to move the (frozen) player to the new gate between rounds. With
// untilBotDead it returns as soon as the current bot's HP reaches 0. Returns false if
// the session was stopped (player quit, cmd 191).
func (s *session) streamPhase(phase uint16, dur time.Duration, pull *message.Vec3, untilBotDead bool) bool {
	gri := message.CSGRIInit(maxRound, uint8(s.round-1))
	param := s.gameParam(dur) // fixed phase-end deadline so the client countdown ticks
	start := time.Now()

	if pull != nil {
		s.tpSeq++
		s.sendData(packet.CmdTeleport, message.ForceTeleport(playerEntityID, s.tpSeq, *pull, s.player.SpawnFace, 0))
	}

	for time.Since(start) < dur {
		if s.stopped.Load() {
			log.Printf("[mm-udp] CS sync loop stopped (player quit) %v", s.remote)
			return false
		}
		s.sendVar(packet.CmdPRISync, s.priPayload(), 1)
		s.sendVar(packet.CmdGRISync, gri, 1)
		s.sendVar(packet.CmdGRISync, message.CSGRIPhase(phase, param), 1)

		if untilBotDead && (s.entityHP(s.botEntity) == 0 || s.entityHP(playerEntityID) == 0) {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return true
}

// serverTimeLoop streams cmd 1000 (SyncServerTime) so the client's match clock
// (CurrentServerTime = ServerGameTickCount/30 s) tracks our matchStart clock — the base
// every CS countdown subtracts from (SafeZone EndTime, GRI phase param). Without it the
// client clock is unsynced and the timers read a garbage default ("12:15"). 30 Hz tick.
func (s *session) serverTimeLoop() {
	for !s.stopped.Load() {
		tick := uint32(time.Since(s.matchStart).Seconds() * 30)
		s.write(&packet.Packet{
			SendOption: packet.SendUnreliable,
			Cmd:        packet.CmdSyncServerTime,
			Flags:      0,
			Payload:    message.SyncServerTime(tick),
		})
		// Re-anchor the client clock often: between ticks the client advances CurrentServerTime
		// on its own (local sim), so a slow interval lets it drift behind and the safe-zone
		// circle it renders lags our shrink. 250ms keeps that drift small (see zoneClientLag).
		time.Sleep(250 * time.Millisecond)
	}
}

// gameParam returns the value the client wants in the GRI phase field's low 16 bits:
// an absolute time in SECONDS on the CS match clock at which the current phase ends.
// The client renders the countdown as (param - GameTime), so we set param to
// secondsSinceMatchStart + the phase's remaining seconds. GameTime is the client's own
// simulation clock; we approximate it with matchStart (a few seconds of startup skew).
func (s *session) gameParam(hold time.Duration) uint16 {
	total := time.Since(s.matchStart).Seconds() + hold.Seconds()
	if total < 0 {
		total = 0
	}
	if total > 65000 { // keep within uint16 (Fight/long holds read the timer elsewhere)
		total = 65000
	}
	return uint16(total)
}

// priPayload builds the PRI (cmd 900) block for the local player (HP + live coins +
// faction) and, while alive, the current-round bot (HP + opposite faction). A dead bot
// is dropped from the stream so its death (cmd 107) is not undone by a fresh HP update.
func (s *session) priPayload() []byte {
	s.invMu.Lock()
	coins := uint16(s.coins)
	award := uint16(s.award)
	s.invMu.Unlock()
	// CS round-win score (field 28), packed from each entity's OWN team perspective so
	// the client resolves my/oppo consistently whichever player's score changes.
	localScore := message.PackScore(s.teamScore[0], s.teamScore[1])
	botScore := message.PackScore(s.teamScore[1], s.teamScore[0])
	ents := []message.PRIEntity{
		{RepID: playerRepID, Block: message.PRIHPBlock(s.entityHP(playerEntityID), maxHP, coins, award, localScore, localFaction)},
	}
	if s.entityHP(s.botEntity) > 0 {
		ents = append(ents, message.PRIEntity{RepID: botRepID, Block: message.PRIHPBlock(s.entityHP(s.botEntity), maxHP, 0, 0, botScore, botFaction)})
	}
	return message.SyncPRI(ents)
}

// shopWarmup holds the match in Waiting for a few seconds while the battle scene reaches
// Running, then populates the shop (cmd 407) and gives the starting loadout, before the
// round loop opens the first buy phase.
func (s *session) shopWarmup() {
	gri := message.CSGRIInit(maxRound, 0)
	start := time.Now()
	for time.Since(start) < 3*time.Second {
		if s.stopped.Load() {
			return
		}
		s.sendVar(packet.CmdPRISync, s.priPayload(), 1)
		s.sendVar(packet.CmdGRISync, gri, 1)
		s.sendVar(packet.CmdGRISync, message.CSGRIPhase(message.CSPhaseWaiting, 0), 1)
		time.Sleep(300 * time.Millisecond)
	}
	s.sendCSShop()
	s.sendStartingLoadout()
}

// sendStartingLoadout gives the player their initial inventory at match start (seeding
// the economy). See giveLoadout.
func (s *session) sendStartingLoadout() { s.giveLoadout(true) }

// giveLoadout (re-)issues the base USP loadout via cmd 174: a USP (data 3) in the
// secondary/pistol slot, held in hand, plus pistol ammo (data 202). At match start
// (resetCoins) it also seeds the economy; on a REVIVE (resetCoins=false) it re-gives the
// weapon the death dropped WITHOUT touching the accumulated coins — the client clears the
// loadout to fists on death, so this restores it (uidCounter resets so the uids match a
// cleared inventory, no duplicate). Purchased weapons are lost on death; the player
// re-buys with their kept coins.
func (s *session) giveLoadout(resetCoins bool) {
	const uspData = 3
	const pistolAmmo, pistolAmmoCount = 202, 120

	s.invMu.Lock()
	if resetCoins {
		s.coins = startingCoins
	}
	s.uidCounter = 0
	uspUID := s.nextUIDLocked()
	ammoUID := s.nextUIDLocked()
	s.equipment = []message.Equipment{
		{Slot: message.SlotMelee, Data: 0, Unique: 0},                // fist (melee slot must always exist)
		{Slot: message.SlotSecondary, Data: uspData, Unique: uspUID}, // USP
	}
	s.itemOnHand = uspUID
	inv := []message.InvItem{
		{Unique: uspUID, Data: uspData, Count: 1, Runtime: 12},      // USP weapon
		{Unique: ammoUID, Data: pistolAmmo, Count: pistolAmmoCount}, // pistol ammo
	}
	equip := append([]message.Equipment(nil), s.equipment...)
	onHand := s.itemOnHand
	s.invMu.Unlock()

	body := message.SyncInventory(s.player.EntityID, inv, nil, equip, onHand)
	s.sendDataLog(packet.CmdSyncInventory, body, "cmd=174 SyncInventory (USP + ammo)")
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
		{RepID: botRepID, EntityType: message.BindEntityPlayer, EntityGameID: s.botEntity},
	})
	s.sendDataLog(packet.CmdBindPRI, bind,
		fmt.Sprintf("cmd=118 BindPRI local ent=%#x + bot ent=%#x", uint32(playerEntityID), s.botEntity))
}
