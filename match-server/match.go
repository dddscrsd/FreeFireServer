package main

import (
	"fmt"
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
	s          *session   // the primary (first) human, == players[0]; single-client paths still read it
	players    []*session // the match roster (humans), filled at join by addPlayer; broadcasts fan over it
	nextSlot   byte       // monotonic participant-slot allocator (allocSlot): slot 1 = 1st human, 2 = bot
	reserved   int        // slots handed out (manager-owned under matchManager.mu) — race-safe routing view
	phase      csPhase
	deadline   time.Time     // when the current phase transitions; a holdDur (far-future) deadline = condition-based (Fight) / hold
	pull       *message.Vec3 // teleport target streamed on a phase entry (frozen-player pin), or nil
	revive     bool          // set at the Fight edge: the local player died -> revive path in the transition
	matchWon   bool          // set at the Fight edge when the match ends: local team won
	winnerTeam byte          // team that won the match — drives the PER-PLAYER cmd 103 rank (each side gets its own win/lose)
	mailbox    chan func()   // inbound gameplay handlers run here (Step 3b); one inbox per match (Step 4)
	done       chan struct{}

	// Shared world state (moved off the session in Step 4a — the whole match sees ONE of each).
	round      int               // 1-based CS round
	teamScore  [2]uint8          // rounds won: [0]=local (faction 0), [1]=enemy (faction 1)
	matchStart time.Time         // CS clock origin (phase countdowns read param - secondsSinceMatchStart)
	tpSeq      uint32            // force-teleport token sequence (round-transition repositions)
	arena      csArena           // spawn city for the current round (re-picked each round)
	bot        joinPlayer        // the current-round enemy bot (becomes a roster *player in 4c)
	botEntity  uint32            // the (fixed) enemy bot entity id
	hp         map[uint32]uint16 // entity id -> current HP (player + bot); splits to per-player in 4c
	kills      map[uint32]uint16 // entity id -> match kill count (scoreboard, PRI field 10)
	deaths     map[uint32]uint16 // entity id -> match death count (PRI field 29)
	damage     map[uint32]uint32 // entity id -> total damage dealt (PRI field 31)

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

	// Multiplayer death rules (Step 5): per-entity lifecycle + live knock records, keyed like
	// m.hp so humans and the bot are handled uniformly. lastTeamFell / roundOver drive round-end.
	life         map[uint32]lifeState    // entity id -> ALIVE|KNOCKED|DEAD (absent == ALIVE)
	knock        map[uint32]*knockState  // entity id -> live knockdown record (only while KNOCKED)
	rescues      map[uint32]*rescueState // target entity id -> in-progress teammate revive (cmd 142..140)
	lastTeamFell byte                    // team whose alive-count most recently hit 0 (both-0 tiebreak)
	roundOver    bool                    // set when a team hits 0 alive; gates further damage this round

	// Match lifecycle: ended is set when the deciding round finishes so join() skips this match (a
	// rejoin gets a fresh one); after a stats-hold the loop closes done, returns, and reap()s itself.
	ended       bool
	tearingDown bool
	revived     []*session // players revived this round-transition — they get a fresh base loadout; survivors keep theirs
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

	matchStatsHold = 20 * time.Second // hold on the end-of-match stats screen before tearing the match down (so a rejoin gets a fresh match)
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
	m := &Match{s: s, mailbox: make(chan func(), 256), done: make(chan struct{}, 2)}
	s.match = m
	return m
}

// allocSlot hands out the next participant's ids. The slot is a monotonic counter (unique entity-id
// low bits); the TEAM is BALANCED — the joiner fills the team with fewer HUMANS (tie -> team 1). The
// bot isn't in m.players, so it doesn't skew the count: the 1st human -> team 1, the bot slot -> team
// 2, and a 2nd human -> team 2 (opposite the 1st). (Was odd/even, which put both humans on team 1.)
// entity id = team<<24 | slot; RepID = playerRepID + slot - 1.
func (m *Match) allocSlot() (entityID, repID uint32, faction, team, idx, slot byte) {
	m.nextSlot++
	slot = m.nextSlot
	var human [3]int // human[1], human[2] = humans currently on team 1 / team 2
	for _, p := range m.players {
		human[p.team]++
	}
	team = 1
	if human[2] < human[1] {
		team = 2
	}
	faction = team - 1
	idx = byte(human[team]) // 0-based index among CURRENT teammates (the retired bot isn't counted)
	entityID = uint32(team)<<24 | uint32(slot)
	repID = playerRepID + uint32(slot) - 1
	return
}

// retireBot removes the bring-up filler bot once a 2nd human is present (plan §2): despawn its pawn on
// the existing clients (cmd 107, no killer) and clear it from all match state so the humans face each
// other on opposing teams. After this m.botEntity == 0 and every bot special-case gates off.
func (m *Match) retireBot() {
	bot := m.botEntity
	m.broadcastData(packet.CmdDead, message.PlayerDead(bot, 0, 0, 0, 0, 0, m.bot.SpawnPos, false),
		fmt.Sprintf("cmd=107 BOT retired ent=%#x (2nd human joined -> human vs human)", bot))
	m.botEntity = 0
	delete(m.hp, bot)
	delete(m.life, bot)
	delete(m.kills, bot)
	delete(m.deaths, bot)
	delete(m.damage, bot)
}

// addPlayer allocates a roster slot for a joining human session — its entity / RepID / faction /
// team from allocSlot — records the ids on the session, and adds it to the match roster. For the
// first human these ids equal the playerEntityID / playerRepID / localFaction constants the game
// logic still reads (the switch to per-session ids is a later step); m.s stays the primary human.
func (m *Match) addPlayer(s *session) {
	s.entityID, s.repID, s.faction, s.team, s.player.TeamIdx, s.slot = m.allocSlot()
	s.player.EntityID = s.entityID
	m.players = append(m.players, s)
	log.Printf("[mm-udp] roster+ slot=%d entity=%#08x rep=%d faction=%d team=%d (roster=%d) %v",
		s.slot, s.entityID, s.repID, s.faction, s.team, len(m.players), s.remote)
}

// removePlayer drops a quitting human from the roster (runs on run()'s mailbox, so it may mutate
// m.players). The match keeps going for the rest; if the roster empties, run() stops on its next tick
// and reaps itself. The leaver's pawn is removed on the other clients via a cmd-107 (no killer / no
// revive), and the primary is reassigned if the one that left was it.
func (m *Match) removePlayer(s *session) {
	if m.sessionByEntity(s.entityID) == nil {
		return // already gone
	}
	m.emitDeath(s.entityID, 0, 0, 0, 0, 0, m.deathPos(s.entityID), false)
	kept := m.players[:0]
	for _, p := range m.players {
		if p != s {
			kept = append(kept, p)
		}
	}
	m.players = kept
	delete(m.hp, s.entityID)
	delete(m.life, s.entityID)
	delete(m.knock, s.entityID)
	if m.s == s && len(m.players) > 0 {
		m.s = m.players[0] // promote a survivor so run()'s primary reads stay valid
	}
	log.Printf("[mm-udp] roster- slot=%d ent=%#x (roster=%d) %v", s.slot, s.entityID, len(m.players), s.remote)
}

// admitFirst runs the join handshake for the FIRST player of a fresh match: pick the arena/spawn,
// seed the round + enemy bot, answer the join (cmd 100 + cmd 101 self + cmd 101 bot + cmd 130),
// draw the zone, and start the match loop. m.arena/round/bot are the match's — set once here. Later
// players (Step 5b) are admitted onto the already-running loop instead of running this.
func (m *Match) admitFirst(s *session) {
	m.setupMatch(s)
	s.sendDataLog(packet.CmdJoinMatchRes, message.JoinMatchRes(0), "cmd=100 LGIGCGIDOKP result=0")
	s.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(s.player),
		fmt.Sprintf("cmd=101 GKBDLJFGGMI acc=%d ent=%d %q", s.player.AccountID, s.player.EntityID, s.player.Name))
	s.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(m.bot),
		fmt.Sprintf("cmd=101 BOT acc=%d ent=%#x %q pos=(%.1f,%.1f,%.1f)",
			m.bot.AccountID, m.bot.EntityID, m.bot.Name, m.bot.SpawnPos.X, m.bot.SpawnPos.Y, m.bot.SpawnPos.Z))
	s.sendDataLog(packet.CmdJoinMatchFinished, message.JoinMatchFinished(), "cmd=130 JoinMatchFinished")
	s.broadcastZone() // draw the safe zone at the NEW city now that the player joined
	s.startCSMatch()
}

// setupMatch initialises a fresh match's world for its first player: pick the arena/spawn, add the
// player to the roster, seed round 1, and create the enemy bot (consuming slot 2 so later humans get
// slot 3+). Shared by admitFirst and the reconnect resume (which skips the join packets).
func (m *Match) setupMatch(s *session) {
	m.arena = choosePlayerSpawn(&s.player)
	m.addPlayer(s)
	m.round = 1
	beid, _, _, _, _, _ := m.allocSlot() // reserve the bot's slot (2) so a 2nd human gets slot 3
	m.botEntity = beid
	m.bot = botPlayer(s.player, m.botEntity)
}

// admitLater admits a 2nd+ human onto the ALREADY-RUNNING match loop (called via run()'s mailbox, so
// it mutates the roster on the single owner). It allocates the player's slot, spawns them at the
// match arena, gives the starting loadout + shop, and does the join handshake: the joiner gets a cmd
// 101 for every existing entity (roster incl. self + bot) + cmd 130; each already-present human gets
// a cmd 101 for the joiner; then everyone re-binds and the joiner sees the zone.
func (m *Match) admitLater(s *session) {
	m.addPlayer(s)
	s.player.SpawnPos, s.player.SpawnFace = m.arena.spawnFor(s.faction)
	s.playerPos = s.player.SpawnPos
	m.hp[s.entityID] = maxHP
	m.life[s.entityID] = lifeAlive

	// The bot was only a 1-human filler; a 2nd human retires it so the humans face each other (§2).
	if m.botEntity != 0 {
		m.retireBot()
	}

	s.sendDataLog(packet.CmdJoinMatchRes, message.JoinMatchRes(0), "cmd=100 join res (2nd+ player)")
	s.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(s.player), // the joiner's OWN entity FIRST (client adopts the first as its local player)
		fmt.Sprintf("cmd=101 self ent=%d %q", s.player.EntityID, s.player.Name))
	for _, p := range m.players {
		if p != s { // then the already-present humans
			s.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(p.player),
				fmt.Sprintf("cmd=101 other ent=%d %q", p.player.EntityID, p.player.Name))
		}
	}
	if m.botEntity != 0 { // only if the bot is still around (1-human match); retired once a 2nd human joins
		s.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(m.bot), fmt.Sprintf("cmd=101 BOT ent=%#x", m.bot.EntityID))
	}
	for _, p := range m.players { // the joiner learns each existing player's inventory (held weapon + skin)
		if p != s {
			s.sendDataLog(packet.CmdSyncInventory, p.currentInventorySync(),
				fmt.Sprintf("cmd=174 existing ent=%#x inventory -> joiner", p.entityID))
		}
	}
	s.sendDataLog(packet.CmdJoinMatchFinished, message.JoinMatchFinished(), "cmd=130 JoinMatchFinished")

	for _, p := range m.players { // every already-present human learns the joiner
		if p != s {
			p.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(s.player),
				fmt.Sprintf("cmd=101 new player ent=%d %q -> %v", s.player.EntityID, s.player.Name, p.remote))
		}
	}

	m.bindAll()         // everyone re-binds so the cmd 900 PRI routes the new RepID
	s.giveLoadout(true) // the joiner's starting loadout (USP + medkits + attachments + coins)
	s.sendCSShop()      // and the shop, so they can buy
	s.broadcastZone()   // draw the current safe zone for the joiner
	log.Printf("[mm-udp] admitted player slot=%d ent=%#08x (roster=%d) %v", s.slot, s.entityID, len(m.players), s.remote)
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
	defer matchManager.reap(m) // remove this match from the registry when the loop exits
	s := m.s
	coins := uint32(startingCoins)
	if cfg.unlimitedMoneyTest {
		coins = 9999
	}
	s.coins = coins

	m.matchStart = time.Now()
	m.initHP()

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
			if len(m.players) == 0 { // every human left -> stop the loop (the defer reaps the match)
				log.Printf("[mm-udp] CS match loop stopped: roster empty")
				return
			}
			// Advance the phase machine every base tick so a transition lands within ~baseTick
			// of its deadline; force the stream on a transition so the new phase's first GRI is
			// not a stream interval behind (this is what made Prepare->Fight look ~1s late).
			before := m.phase
			m.advancePhase(now)
			if m.phase != before || now.Sub(lastStream) >= priTick {
				m.stream()
				m.s.sweepWalls() // force-break any gloo wall past its lifetime
				lastStream = now
			}
			m.stepZone(now)
			m.stepKnock(now)   // bleed any downed players; a bleed-out finalizes a cmd-107 death
			m.stepRescue(now)  // complete / cancel in-progress teammate revives
			m.streamMovement() // Step 6: relay each human's latest cmd-1001 state to the others
			for _, p := range m.players {
				p.stepHeal(now) // medkit heal-over-time, per player (run()-driven)
				p.stepEP(now)   // mushroom EP -> HP regen, per player
			}
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

// streamMovement relays each human's latest cmd-1001 state to the OTHER players (Step 6): one batch
// per sender, sent only to the others (no self-echo, so no skip-own is needed). The bot is static
// and isn't relayed — the client keeps it at its cmd-101 spawn. The seq is the subtle part (below).
func (m *Match) streamMovement() {
	matchTime := int32(time.Since(m.matchStart).Seconds() * 30)
	for _, sender := range m.players {
		if len(sender.moveBody) != moveBodyLen || sender.moveBase == 0 {
			continue
		}
		if sender.moveBaseTick == 0 {
			sender.moveBaseTick = matchTime // capture on the first relay so the first relayed seq == moveBase
		}
		// Seq = a LARGE per-player base (the first send-seq, clears the per-pawn gate whose stored seq
		// inits ~1.78e9) advanced only by the 30Hz match-tick DELTA. The client interpolates over
		// (newSeq-oldSeq) FRAMES with no clamp for remotes, so the delta must be small (~3 per 10Hz
		// relay = the relay interval); the raw send-seq's ~99/update delta was the coast-after-stop.
		seq := sender.moveBase + uint32(matchTime-sender.moveBaseTick)
		payload := message.MoveBatch(matchTime, seq, []message.MoveEntry{{Slot: sender.slot, Body: sender.moveBody}})
		pkt := &packet.Packet{SendOption: packet.SendUnreliable, Cmd: packet.CmdClientPos, Flags: 0, Payload: payload}
		for _, recv := range m.players {
			if recv != sender && recv.out != nil {
				recv.out.send(pkt, "")
			}
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
	for _, p := range m.players {
		m.tpSeq++
		m.broadcastData(packet.CmdTeleport, message.ForceTeleport(p.entityID, m.tpSeq, p.player.SpawnPos, p.player.SpawnFace, 0), "")
	}
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
		if m.teamAlive(localTeamID) == 0 || m.teamAlive(enemyTeamID) == 0 {
			m.endFight(now) // a whole team is down (dead / knocked-then-swept) -> round over
		}
		return
	}
	if m.deadline.IsZero() || now.Before(m.deadline) {
		return
	}
	switch m.phase {
	case phSetupBind:
		m.bindAll()
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
		m.matchEnd()
		m.ended = true                           // join() now skips this match; a rejoin starts a fresh one
		m.enter(now, phMatchEnd, matchStatsHold) // hold on the stats screen, then tear down
	case phPostBanner:
		m.postBannerEdge(now)
	case phPostReviveBind:
		m.bindAll() // rebind everyone after the revives so the streamed PRI HP lands on the pawns
		reset := map[*session]bool{}
		for _, p := range m.revived {
			reset[p] = true
		}
		for _, p := range m.players {
			if reset[p] {
				p.giveLoadout(false) // died -> lost its loadout; re-give the base USP (keeps coins)
			} else {
				p.reissueLoadout() // survived -> KEEP the loadout, just refill magazines
			}
		}
		m.revived = nil
		m.enter(now, phPostBlack, postBlackHold)
		m.sendPull() // reposition every player to its new-round spawn
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
		if !m.tearingDown { // stats-hold elapsed -> stop the loop; run()'s defer reaps the match
			m.tearingDown = true
			close(m.done)
		}
	}
}

// endFight scores the finished round (kills + win bonus into coins), then either ends the match
// (phMatchEndPause -> cmd 103) or starts the between-rounds transition (phPostBanner). Replaces
// the csSyncLoop scoring block + roundTransition's setup step.
func (m *Match) endFight(now time.Time) {
	s := m.s
	m.stopZone()
	m.roundOver = true
	localWon := m.teamAlive(localTeamID) > 0 // the surviving team won (sequential falls => first-to-fall loses)
	winnerTeam := byte(localTeamID)
	if localWon {
		m.teamScore[0]++
	} else {
		winnerTeam = enemyTeamID
		m.teamScore[1]++
	}
	// Per-player round coins: each player earns off THEIR OWN kills + a round bonus, plus a win
	// bonus for the WINNING team's members (was m.s-only, so only player 1 got paid).
	for _, p := range m.players {
		aw := 500 * p.roundKills.Swap(0)
		aw += 500 * uint32(m.round)
		if p.team == winnerTeam {
			aw += 500 // win bonus
		}
		p.coins += aw
		p.award = aw
	}
	log.Printf("[mm-udp] ROUND %d won by team %d (score %d-%d)", m.round, winnerTeam, m.teamScore[0], m.teamScore[1])

	if m.teamScore[0] >= roundsToWin || m.teamScore[1] >= roundsToWin || m.round >= maxRound {
		m.matchWon = localWon
		m.winnerTeam = winnerTeam                    // the per-player cmd 103 rank reads this so each side gets its OWN win/lose
		m.enter(now, phMatchEndPause, matchEndPause) // pause on the final kill, then cmd 103
		return
	}
	s.roundResult(winnerTeam) // cmd 409 round result (non-deciding rounds only)
	m.revive = !localWon
	// Transition setup (was roundTransition step 1): pick the next arena + spawns + a fresh bot.
	m.arena = pickArena()
	for _, p := range m.players {
		p.player.SpawnPos, p.player.SpawnFace = m.arena.spawnFor(p.faction)
	}
	if m.botEntity != 0 {
		m.bot = botPlayer(s.player, m.botEntity) // same entity, new spot near the primary player
	}
	log.Printf("[mm-udp] round transition -> arena=%q revivePlayer=%v %v", m.arena.City, m.revive, s.remote)
	m.enter(now, phPostBanner, postToBlack)
}

// postBannerEdge fires when the banner+fade window ends (screen fully black): reset HP,
// reposition unseen, respawn the bot, and either revive the dead local player (revive path) or
// teleport the surviving player + refill its ammo (won path). Was roundTransition steps 3-5.
func (m *Match) postBannerEdge(now time.Time) {
	s := m.s
	dead := m.deadHumans() // capture BEFORE reviveAll flips everyone back to ALIVE
	m.reviveAll()          // reset EVERY roster human + the bot to full HP + ALIVE, clear all knocks
	for _, p := range m.players {
		p.playerPos = p.player.SpawnPos // reset so the shrinking zone can't damage a stale position
		p.resetEP()                     // fresh round: clear the mushroom EP buffer + per-round count
	}
	s.clearWalls() // gloo walls are per-round world state
	if m.botEntity != 0 {
		s.respawnBot()
	}
	for _, p := range dead {
		m.respawnPlayer(p) // cmd 388: revive each dead human at its new spawn (broadcast)
	}
	spawn := s.player.SpawnPos
	m.pull = &spawn // set = "reposition active"; sendPull teleports EVERY player to its own new gate
	if len(dead) > 0 {
		m.revived = dead // only the players who died get a fresh loadout; survivors keep theirs
		m.enter(now, phPostReviveBind, bindDelay)
		return
	}
	for _, p := range m.players {
		p.reissueLoadout() // nobody died: refill every survivor's magazine + reserve
	}
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
	if cfg.zoneStatic {
		waitDur = holdDur // never shrinks: a fixed damaging circle (walk out of it to trigger a knockdown)
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
	for _, p := range m.players {
		if m.lifeOf(p.entityID) != lifeAlive {
			continue // dead / knocked players don't take zone damage
		}
		pos := p.playerPos
		if d := dist2D(pos, m.zoneCenter); d > curR {
			log.Printf("[mm-udp] ZONE damage: %#x %.0fm out (r=%.0f) at (%.0f,%.0f) -> -%d HP %v",
				p.entityID, d-curR, curR, pos.X, pos.Z, zoneDamage, p.remote)
			m.applyDamage(p.entityID, 0, zoneDamage, 0) // killer 0 -> environment zone death
		}
	}
}
