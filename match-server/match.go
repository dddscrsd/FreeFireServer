package main

import (
	"log"
	"time"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// Match owns one CS match's single tick loop + deadline-driven phase state machine. It
// replaces the blocking csSyncLoop / streamPhase / serverTimeLoop / runSafeZone / roundTransition
// control flow (MULTIPLAYER_PLAN.md Step 3a). In 3a it wraps the single session as its one
// player; run() reads/writes session state under the session's EXISTING mutexes (handlers
// unchanged) — 3b routes handlers through a mailbox and drops the mutexes, Step 4 splits this
// into Match + Player + a real roster.
type Match struct {
	s        *session   // the primary (first) human, == players[0]; single-client paths still read it
	players  []*session // the match roster (humans), filled at join by addPlayer; broadcasts fan over it
	nextSlot byte       // monotonic participant-slot allocator (allocSlot): slot 1 = 1st human, 2 = bot
	phase    csPhase
	deadline time.Time     // when the current phase transitions; a holdDur (far-future) deadline = condition-based (Fight) / hold
	pull     *message.Vec3 // teleport target streamed on a phase entry (frozen-player pin), or nil
	revive   bool          // set at the Fight edge: the local player died -> revive path in the transition
	matchWon bool          // set at the Fight edge when the match ends: local team won
	mailbox  chan func()   // inbound gameplay handlers run here (Step 3b); one inbox per match (Step 4)
	done     chan struct{}

	// Shared world state (moved off the session in Step 4a — the whole match sees ONE of each).
	round      int               // 1-based CS round
	teamScore  [2]uint8          // rounds won: [0]=local (faction 0), [1]=enemy (faction 1)
	matchStart time.Time         // CS clock origin (phase countdowns read param - secondsSinceMatchStart)
	tpSeq      uint32            // force-teleport token sequence (round-transition repositions)
	arena      csArena           // spawn city for the current round (re-picked each round)
	bot        joinPlayer        // the current-round enemy bot (becomes a roster *player in 4c)
	botEntity  uint32            // the (fixed) enemy bot entity id
	hp         map[uint32]uint16 // entity id -> current HP (player + bot); splits to per-player in 4c

	// Shared world objects: placed gloo walls (FIFO, walls[0] oldest) + ground-loot boxes.
	walls           []*iceWall
	wallSeq         uint32                // monotonic wall entity-id / PRI-RepID allocator (never reused)
	containers      map[uint16]*container // live ground boxes keyed by ContainerObjectID
	nextContainerID uint16                // runtime container-id allocator (runtimeContainerBase..Max)

	// Zone state (during Fight), replacing runSafeZone's goroutine + tickers.
	zoneOn          bool
	zoneCenter      message.Vec3
	zoneOuterR      float64
	zoneInnerR      float64
	zoneWaitEnd     time.Time // wait stage ends (shrink begins) here
	zoneShrink      bool
	zoneShrinkStart time.Time
	zoneLastDmg     time.Time // last out-of-zone damage tick (2s cadence)
}

// csPhase is a state in the match loop. Each maps to a client GRI phase (griPhase) and has a
// deadline; advancePhase fires the edge when the deadline passes (or "enemy dead" for Fight).
type csPhase int

const (
	phSetupBind      csPhase = iota // +bindDelay -> bindPRIs
	phSetupGap                      // +setupGapDur -> start the Waiting stream
	phSetupWait                     // +setupWaitDur Waiting stream -> shop + starting loadout -> Prepare
	phPrepare                       // +buyPhase (or hold) -> Fight
	phFight                         // enemy/player dead -> score -> MatchEndPause or PostBanner
	phMatchEndPause                 // +matchEndPause -> matchEnd (cmd 103) -> MatchEnd
	phPostBanner                    // +postToBlack (Post) -> reposition/respawn -> ReviveBind or PostBlack
	phPostReviveBind                // +bindDelay -> bindPRIs + giveLoadout -> PostBlack
	phPostBlack                     // +postBlackHold (Post, teleport on entry) -> round++ + zone -> IntroWait
	phIntroWait                     // +bindDelay -> Intro
	phIntro                         // +introReveal+introSettle (Introduction, teleport on entry) -> Prepare
	phMatchEnd                      // hold (Post) — the match is over
)

const (
	baseTick      = 100 * time.Millisecond  // loop wake period — a phase transition lands within ~baseTick of its deadline
	priTick       = 300 * time.Millisecond  // GRI/PRI stream throttle (matches the old 300ms; also forced on a phase transition)
	clockTick     = 250 * time.Millisecond  // cmd 1000 server-clock throttle (matches the old serverTimeLoop)
	setupGapDur   = 1000 * time.Millisecond // bindPRIs -> Waiting gap (was the csSyncLoop sleep)
	setupWaitDur  = 3000 * time.Millisecond // Waiting stream before the shop opens (was shopWarmup)
	bindDelay     = 150 * time.Millisecond  // small settle before a rebind / the intro reveal
	matchEndPause = 500 * time.Millisecond  // pause on the final kill before cmd 103
	introSettle   = 1000 * time.Millisecond // extra Intro hold (folded in from the post-transition sleep)
	holdDur       = 6 * time.Hour           // Fight/MatchEnd/held-Prepare: a fixed far-future deadline so phaseParam stays CONSTANT across the phase (a drifting param makes the client re-fire the round-start each tick)
)

// griPhase maps a csPhase to the client GRI phase to stream, or ok=false to stream nothing
// (clock only) — the setup gaps and the intro-wait window are silent, matching the old code.
func griPhase(ph csPhase) (uint16, bool) {
	switch ph {
	case phSetupWait:
		return message.CSPhaseWaiting, true
	case phPrepare:
		return message.CSPhasePrepare, true
	case phFight, phMatchEndPause:
		return message.CSPhaseFight, true
	case phPostBanner, phPostReviveBind, phPostBlack, phMatchEnd:
		return message.CSPhasePost, true
	case phIntro:
		return message.CSPhaseIntroduction, true
	default: // phSetupBind, phSetupGap, phIntroWait -> silent (clock only)
		return 0, false
	}
}

// newMatch creates the Match a session belongs to, wiring the back-reference both ways. In Step 4
// it is created 1:1 with the session (from main.go); Step 4b hands match creation to the
// MatchManager so multiple players can share one match.
func newMatch(s *session) *Match {
	m := &Match{s: s, mailbox: make(chan func(), 256), done: make(chan struct{})}
	s.match = m
	matchManager.register(m)
	return m
}

// allocSlot hands out the next participant's ids from the match's monotonic slot counter, so no
// two players collide. Odd slot -> team 1 (faction 0, the "local"/left-gate side), even slot ->
// team 2 (faction 1, right gate). It reproduces the old constants exactly: slot 1 = entity
// 0x01000001 / RepID 1000 / faction 0 (the first human), slot 2 = entity 0x02000002 / RepID 1001 /
// faction 1 (the bot). entity id = team<<24 | slot; RepID = playerRepID + slot - 1.
func (m *Match) allocSlot() (entityID, repID uint32, faction, team, slot byte) {
	m.nextSlot++
	slot = m.nextSlot
	team = 2 - slot%2
	faction = team - 1
	entityID = uint32(team)<<24 | uint32(slot)
	repID = playerRepID + uint32(slot) - 1
	return
}

// addPlayer allocates a roster slot for a joining human session — its entity / RepID / faction /
// team from allocSlot — records the ids on the session, and adds it to the match roster. For the
// first human these ids equal the playerEntityID / playerRepID / localFaction constants the game
// logic still reads (the switch to per-session ids is a later step); m.s stays the primary human.
func (m *Match) addPlayer(s *session) {
	s.entityID, s.repID, s.faction, s.team, s.slot = m.allocSlot()
	s.player.EntityID = s.entityID
	m.players = append(m.players, s)
	log.Printf("[mm-udp] roster+ slot=%d entity=%#08x rep=%d faction=%d team=%d (roster=%d) %v",
		s.slot, s.entityID, s.repID, s.faction, s.team, len(m.players), s.remote)
}

// startCSMatch launches the match's tick loop once per session. Safe to call from both the
// fresh-join and the mid-match reconnect paths (replaces startCSSync).
func (s *session) startCSMatch() {
	if s.syncStarted {
		return
	}
	s.syncStarted = true
	log.Printf("[mm-udp] -> starting CS match loop (base %v, stream %v, clock %v) %v", baseTick, priTick, clockTick, s.remote)
	go s.match.run()
}

// run is the single loop: each base tick it advances the phase machine and steps the zone, and
// on its own throttle it streams the current phase (GRI+PRI) + the server clock. A phase
// transition forces the stream so the new phase's first GRI lands same-tick. Replaces
// csSyncLoop + streamPhase + serverTimeLoop + runSafeZone.
func (m *Match) run() {
	s := m.s
	coins := uint32(startingCoins)
	if cfg.unlimitedMoneyTest {
		coins = 9999
	}
	s.coins = coins

	m.matchStart = time.Now()
	if m.round == 0 { // reconnect path skips the join handler that seeds round 1
		m.round = 1
		m.botEntity = botEntityID
	}
	s.initHP()

	m.enter(time.Now(), phSetupBind, bindDelay)
	m.streamClock() // seed the client clock at t=0 (the old serverTimeLoop sent one immediately)

	t := time.NewTicker(baseTick)
	defer t.Stop()
	lastStream := time.Time{} // zero -> stream + sweep on the first tick
	lastClock := time.Now()   // the clock was just seeded; the next one is ~clockTick out

	for {
		select {
		case <-m.done:
			return
		case fn := <-m.mailbox: // an inbound gameplay handler — runs here so run() owns all match state
			fn()
		case now := <-t.C:
			if s.stopped.Load() {
				log.Printf("[mm-udp] CS match loop stopped (player quit) %v", s.remote)
				return
			}
			// Advance the phase machine every base tick so a transition lands within ~baseTick
			// of its deadline; force the stream on a transition so the new phase's first GRI is
			// not a stream interval behind (this is what made Prepare->Fight look ~1s late).
			before := m.phase
			m.advancePhase(now)
			if m.phase != before || now.Sub(lastStream) >= priTick {
				m.stream()
				s.sweepWalls() // force-break any gloo wall past its lifetime
				lastStream = now
			}
			m.stepZone(now)
			s.stepHeal(now) // medkit heal-over-time (run()-driven; was the healMedkit goroutine)
			if now.Sub(lastClock) >= clockTick {
				m.streamClock()
				lastClock = now
			}
		}
	}
}

// stream broadcasts the current phase's GRI + PRI for one tick (nothing during the silent
// setup/intro-wait windows). Mirrors the old streamPhase inner body; sendVar fans out (Step 2).
func (m *Match) stream() {
	s := m.s
	phase, ok := griPhase(m.phase)
	if !ok {
		return // silent window (clock only)
	}
	param := m.phaseParam()
	if m.phase == phSetupWait {
		param = 0 // the Waiting hold shows no countdown (matches the old shopWarmup)
	}
	s.sendVar(packet.CmdPRISync, m.priPayload(), 1)
	s.sendVar(packet.CmdGRISync, message.CSGRIInit(maxRound, uint8(m.round-1)), 1)
	s.sendVar(packet.CmdGRISync, message.CSGRIPhase(phase, param), 1)
	point := 0
	if m.teamScore[0] == roundsToWin-1 || m.teamScore[1] == roundsToWin-1 {
		point = 1 // next round is the decider
	}
	s.sendVar(packet.CmdGRISync, message.CSGRIMatchPoint(uint8(point)), 1)
}

// streamClock streams cmd 1000 (server clock) to every human in the roster so the CS countdowns
// tick. It is SendUnreliable + plaintext (no per-connection seq), so the same packet fans to all.
// Replaces serverTimeLoop.
func (m *Match) streamClock() {
	tick := uint32(time.Since(m.matchStart).Seconds() * 30)
	pkt := &packet.Packet{SendOption: packet.SendUnreliable, Cmd: packet.CmdSyncServerTime, Flags: 0, Payload: message.SyncServerTime(tick)}
	for _, p := range m.players {
		if p.out != nil {
			p.out.send(pkt, "")
		}
	}
}

// rosterWriters returns the Writer of every human in the roster (bots have out==nil, skipped) —
// the fan-out target for the VAR streaming broadcasts (see sendVar).
func (m *Match) rosterWriters() []*Writer {
	outs := make([]*Writer, 0, len(m.players))
	for _, p := range m.players {
		if p.out != nil {
			outs = append(outs, p.out)
		}
	}
	return outs
}

// phaseParam is the GRI phase-field value: the match-clock second at which the current phase
// ends (the client renders the countdown as param - GameTime), derived from the deadline.
// Condition/hold phases carry a holdDur (far-future) deadline, so this value stays CONSTANT
// across the phase's ticks — a drifting param makes the client re-fire the phase/round start
// every tick. Same math as the old gameParam. The IsZero branch is a defensive fallback.
func (m *Match) phaseParam() uint16 {
	var endSec float64
	if m.deadline.IsZero() {
		endSec = time.Since(m.matchStart).Seconds() + (6 * time.Hour).Seconds()
	} else {
		endSec = m.deadline.Sub(m.matchStart).Seconds()
	}
	if endSec < 0 {
		endSec = 0
	}
	if endSec > 65000 { // keep within uint16
		endSec = 65000
	}
	return uint16(endSec)
}

// enter sets the current phase + its deadline (dur<=0 => no deadline: condition-based Fight or a
// hold). It does NOT send the entry teleport — callers that need one call sendPull after.
func (m *Match) enter(now time.Time, ph csPhase, dur time.Duration) {
	m.phase = ph
	if dur <= 0 {
		m.deadline = time.Time{}
	} else {
		m.deadline = now.Add(dur)
	}
}

// sendPull streams the force-teleport (cmd 145) that pins the frozen local player to m.pull,
// if one is set — used when entering the black-hold/intro phases (was streamPhase's pull).
func (m *Match) sendPull() {
	if m.pull == nil {
		return
	}
	s := m.s
	m.tpSeq++
	s.sendData(packet.CmdTeleport, message.ForceTeleport(playerEntityID, m.tpSeq, *m.pull, s.player.SpawnFace, 0))
}

// enterPrepare opens a buy phase (or holds it forever in the dev holdPrepare mode).
func (m *Match) enterPrepare(now time.Time) {
	m.pull = nil
	dur := cfg.buyPhase
	if cfg.holdPrepare { // dev shop-testing: never leave Prepare
		dur = holdDur
	}
	m.enter(now, phPrepare, dur)
}

// startFight begins the Fight phase (ends on the enemy/player-dead condition, not the deadline)
// and starts the safe zone. holdDur only feeds phaseParam a stable countdown; advancePhase ends
// Fight via the condition check, which runs before the deadline check.
func (m *Match) startFight(now time.Time) {
	m.enter(now, phFight, holdDur)
	m.startZone()
}

// advancePhase runs the state machine each tick: Fight ends on a condition, every other phase
// on its deadline. Each edge fires the side effects + moves to the next phase.
func (m *Match) advancePhase(now time.Time) {
	s := m.s
	if m.phase == phFight { // condition-based, not a deadline
		if m.entityHP(m.botEntity) == 0 || m.entityHP(playerEntityID) == 0 {
			m.endFight(now)
		}
		return
	}
	if m.deadline.IsZero() || now.Before(m.deadline) {
		return
	}
	switch m.phase {
	case phSetupBind:
		s.bindPRIs()
		m.enter(now, phSetupGap, setupGapDur)
	case phSetupGap:
		m.enter(now, phSetupWait, setupWaitDur)
	case phSetupWait:
		s.sendCSShop()
		s.sendStartingLoadout()
		m.enterPrepare(now)
	case phPrepare:
		m.startFight(now)
	case phMatchEndPause:
		s.matchEnd(m.matchWon)
		m.enter(now, phMatchEnd, holdDur) // hold
	case phPostBanner:
		m.postBannerEdge(now)
	case phPostReviveBind:
		s.bindPRIs()         // rebind after the revive so the streamed PRI HP lands on the pawn
		s.giveLoadout(false) // death dropped the weapon to fists — re-give, keep coins
		m.enter(now, phPostBlack, postBlackHold)
		m.sendPull() // m.pull is nil on the revive path (cmd 388 repositioned)
	case phPostBlack:
		m.round++
		s.broadcastZone() // draw the NEW city's safe zone now the player teleported
		m.enter(now, phIntroWait, bindDelay)
	case phIntroWait:
		m.enter(now, phIntro, introReveal+introSettle)
		m.sendPull()
	case phIntro:
		m.enterPrepare(now) // next round's buy phase
	case phMatchEnd:
		// hold forever
	}
}

// endFight scores the finished round (kills + win bonus into coins), then either ends the match
// (phMatchEndPause -> cmd 103) or starts the between-rounds transition (phPostBanner). Replaces
// the csSyncLoop scoring block + roundTransition's setup step.
func (m *Match) endFight(now time.Time) {
	s := m.s
	m.stopZone()
	localWon := m.entityHP(playerEntityID) > 0
	winnerTeam := byte(localTeamID)
	if localWon {
		m.teamScore[0]++
	} else {
		winnerTeam = enemyTeamID
		m.teamScore[1]++
	}
	award := 500 * s.roundKills.Swap(0)
	award += 500 * uint32(m.round)
	if localWon {
		award += 500 // win bonus
	}
	s.coins += award
	s.award = award
	total := s.coins
	log.Printf("[mm-udp] ROUND %d won by team %d (score %d-%d) +%d coins (=%d) %v",
		m.round, winnerTeam, m.teamScore[0], m.teamScore[1], award, total, s.remote)

	if m.teamScore[0] >= roundsToWin || m.teamScore[1] >= roundsToWin || m.round >= maxRound {
		m.matchWon = localWon
		m.enter(now, phMatchEndPause, matchEndPause) // pause on the final kill, then cmd 103
		return
	}
	s.roundResult(winnerTeam) // cmd 409 round result (non-deciding rounds only)
	m.revive = !localWon
	// Transition setup (was roundTransition step 1): pick the next arena + spawns + a fresh bot.
	m.arena = pickArena()
	s.player.SpawnPos, s.player.SpawnFace = m.arena.spawnFor(localFaction)
	m.bot = botPlayer(s.player, m.botEntity) // same entity, new spot near the player
	log.Printf("[mm-udp] round transition -> arena=%q revivePlayer=%v %v", m.arena.City, m.revive, s.remote)
	m.enter(now, phPostBanner, postToBlack)
}

// postBannerEdge fires when the banner+fade window ends (screen fully black): reset HP,
// reposition unseen, respawn the bot, and either revive the dead local player (revive path) or
// teleport the surviving player + refill its ammo (won path). Was roundTransition steps 3-5.
func (m *Match) postBannerEdge(now time.Time) {
	s := m.s
	m.hp = map[uint32]uint16{playerEntityID: maxHP, m.botEntity: maxHP}
	s.playerPos = s.player.SpawnPos // reset so the shrinking zone can't damage a stale position
	s.clearWalls()                  // gloo walls are per-round world state
	s.respawnBot()
	if m.revive {
		s.respawnLocalPlayer() // cmd 388: clear the dead flag + reposition + un-spectate
		m.pull = nil           // the revive repositioned; no teleport pull
		m.enter(now, phPostReviveBind, bindDelay)
		return
	}
	s.reissueLoadout() // alive: refill every kept weapon's magazine + reserve
	spawn := s.player.SpawnPos
	m.pull = &spawn // pin the frozen survivor to the new gate through the black window + intro
	m.enter(now, phPostBlack, postBlackHold)
	m.sendPull()
}

// startZone begins the Fight safe zone (stage 1: static wait circle) and arms the shrink +
// damage timing. Replaces runSafeZone's setup + goroutine.
func (m *Match) startZone() {
	if cfg.noZone {
		return
	}
	s := m.s
	center, outerR := s.zoneGeometry()
	m.zoneCenter = center
	m.zoneOuterR = outerR
	m.zoneInnerR = outerR * zoneInnerRatio
	waitDur := zoneWaitDur
	if cfg.zoneTest {
		waitDur = 3 * time.Second
	}
	now := time.Now()
	m.zoneWaitEnd = now.Add(waitDur)
	m.zoneShrink = false
	m.zoneLastDmg = now // first out-of-zone damage fires ~zoneDamageEvery from now
	m.zoneOn = true
	s.sendZone(byte(m.round), center, outerR, center, m.zoneInnerR, message.ZoneWaiting, waitDur)
}

// stopZone ends the Fight zone (round over).
func (m *Match) stopZone() { m.zoneOn = false }

// stepZone runs the zone each tick during Fight: flip wait->shrink at the boundary (send the
// shrink cmd 117) and apply out-of-zone damage on the ~2s cadence against the client-lag-
// compensated radius. Replaces runSafeZone's select loop.
func (m *Match) stepZone(now time.Time) {
	if !m.zoneOn {
		return
	}
	s := m.s
	if !m.zoneShrink && !now.Before(m.zoneWaitEnd) { // wait -> shrink boundary
		s.sendZone(byte(m.round), m.zoneCenter, m.zoneOuterR, m.zoneCenter, m.zoneInnerR, message.ZoneShrinking, zoneShrinkDur)
		m.zoneShrink = true
		m.zoneShrinkStart = now
	}
	if now.Sub(m.zoneLastDmg) < zoneDamageEvery {
		return
	}
	m.zoneLastDmg = now
	curR := m.zoneOuterR
	if m.zoneShrink { // the radius the client is STILL rendering (a touch in the past)
		curR = lerpRadius(m.zoneOuterR, m.zoneInnerR, now.Sub(m.zoneShrinkStart)-zoneClientLag)
	}
	pos := s.playerPos
	if d := dist2D(pos, m.zoneCenter); d > curR {
		log.Printf("[mm-udp] ZONE damage: player %.0fm out (r=%.0f) at (%.0f,%.0f) -> -%d HP %v",
			d-curR, curR, pos.X, pos.Z, zoneDamage, s.remote)
		s.applyDamage(playerEntityID, 0, zoneDamage, 0) // killer 0 -> environment zone death
	}
}
