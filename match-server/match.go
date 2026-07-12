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
	lossStreak [2]uint8          // consecutive rounds LOST per team (drives the CS loss-bonus ladder)
	firstBlood uint32            // entity that got THIS round's first cross-team kill (0 = none yet); reset each round in endFight
	reseedCount int              // how many times the cmd-101 roster has been re-emitted for the replay recording (see reseedJoinBurst)
	lastReseed  time.Time        // throttle marker for the reseed window
	wheelKicked bool             // whether the one-shot replay weapon-wheel kick (cmd-108 on-hand toggle) has fired (see kickObserverWheel)
	settings   matchSettings     // per-match CS config (round count / economy); defaults + custom-room overrides
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
	flags           map[uint32]flagObject // owner entityId -> live battle-flag object (1 per player; emote-planted, 60s server-despawn, see battleflag.go)
	flagSeq         uint32                // monotonic flag object-id allocator (flagEntityBase band)

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
	started     bool // proceeded past waiting-for-players -> LOCKED: canAdmit is false, so latecomers get a fresh match
	tearingDown bool
	revived     []*session       // players revived this round-transition — they get a fresh base loadout; survivors keep theirs
	arenasUsed  map[csArena]bool // arena -> true for arenas already used this match (avoid repeats)
}

// csPhase is a state in the match loop. Each maps to a client GRI phase (griPhase) and has a
// deadline; advancePhase fires the edge when the deadline passes (or "enemy dead" for Fight).
type csPhase int

const (
	phSetupBind      csPhase = iota // +bindDelay -> bindPRIs
	phSetupGap                      // +setupGapDur -> start the Waiting stream
	phSetupWait                     // +setupWaitDur brief Waiting intro -> shop + starting loadout -> Prepare (which then holds for players)
	phPrepare                       // +buyPhase (or hold) -> Fight
	phFight                         // enemy/player dead -> score -> MatchEndPause or PostBanner
	phMatchEndPause                 // +matchEndPause -> matchEnd (cmd 103) -> MatchEnd
	phPostBanner                    // +postToBlack (Post) -> reposition/respawn -> ReviveBind or PostBlack
	phPostReviveBind                // +bindDelay -> bindPRIs + giveLoadout -> PostBlack
	phPostBlack                     // +postBlackHold (Post, teleport on entry) -> round++ + zone -> IntroWait
	phIntroWait                     // +bindDelay -> Intro
	phIntro                         // +introReveal+introSettle (Introduction, teleport on entry) -> Prepare
	phMatchEnd                      // hold (Post) — the match is over
	phCancelled                     // pre-match cancel: hold the "Match cancelled" overlay (field 5 CutShort), then ReturnToLobby + reap
)

const (
	baseTick         = 100 * time.Millisecond  // loop wake period — a phase transition lands within ~baseTick of its deadline
	priTick          = 300 * time.Millisecond  // GRI/PRI stream throttle (matches the old 300ms; also forced on a phase transition)
	clockTick        = 250 * time.Millisecond  // cmd 1000 server-clock throttle (matches the old serverTimeLoop)
	clientFixedDelta = 0.033                   // client Time.fixedDeltaTime — the server clock advances 1/this ticks per second, so CurrentServerTime = clientFixedDelta × CEDJCPLOLNE (see serverTick / message.PlayerJoin)
	setupGapDur      = 1000 * time.Millisecond // bindPRIs -> Waiting gap (was the csSyncLoop sleep)
	setupWaitDur     = 3000 * time.Millisecond // brief Waiting intro before the shop opens (was shopWarmup)
	waitPlayersDur   = 30 * time.Second        // "Waiting for players" hold DURING Prepare: proceed EARLY when teams balance (teamsReady), else at this deadline fall back to a bot (MATCH_BOT) or cancel
	cancelHoldDur    = 10 * time.Second        // "Match cancelled" (field 5 CutShort) hold before ReturnToLobby + reap — matches the client's fixed 10s CutShort countdown
	bindDelay        = 150 * time.Millisecond  // small settle before a rebind / the intro reveal
	matchEndPause    = 500 * time.Millisecond  // pause on the final kill before cmd 103
	introSettle      = 1000 * time.Millisecond // extra Intro hold (folded in from the post-transition sleep)
	holdDur          = 6 * time.Hour           // Fight/MatchEnd/held-Prepare: a fixed far-future deadline so phaseParam stays CONSTANT across the phase (a drifting param makes the client re-fire the round-start each tick)

	matchStatsHold = 20 * time.Second // hold on the end-of-match stats screen before tearing the match down (so a rejoin gets a fresh match)
)

// griPhase maps a csPhase to the client GRI phase to stream, or ok=false to stream nothing
// (clock only) — the setup gaps and the intro-wait window are silent, matching the old code.
func griPhase(ph csPhase) (uint16, bool) {
	switch ph {
	case phSetupWait:
		return message.CSPhaseWaiting, true
	case phCancelled:
		// Stay in Prepare (unfrozen, at the gate), NOT Fight: the field-3 phase countdown then renders the
		// 10s cancel timer (param - CurrentServerTime). Fight put the client on the SafeZone round timer,
		// which — with no zone during the cancel — showed a garbage ~34000-minute value. The "Match cancelled"
		// text + its authoritative 10s ride field 5 (state 2 / CutShort).
		return message.CSPhasePrepare, true
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
func (m *Match) allocSlot(prefer byte) (entityID, repID uint32, faction, team, idx, slot byte) {
	m.nextSlot++
	slot = m.nextSlot
	var human [3]int // human[1], human[2] = humans currently on team 1 / team 2
	for _, p := range m.players {
		human[p.team]++
	}
	if prefer == 1 || prefer == 2 {
		team = prefer // custom room: honor the host's arrangement (deterministic team)
	} else {
		team = 1 // matchmaker/queue: balance-fill the emptier team (tie -> team 1)
		if human[2] < human[1] {
			team = 2
		}
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
	if m.botEntity == 0 {
		return
	}

	bot := m.botEntity
	m.broadcastData(packet.CmdDead, message.PlayerDead(bot, 0, 0, 0, 0, 0, m.bot.SpawnPos, true, false),
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
	s.entityID, s.repID, s.faction, s.team, s.player.TeamIdx, s.slot = m.allocSlot(s.player.PreferTeam)
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
	// Tell the OTHERS "this player left" via cmd 102 (RUDP_PLAYER_QUIT) — the client removes just that
	// entity. This was cmd 192 (PLAYER_QUIT_RES), the server's ack of the recipient's OWN quit, so every
	// remaining client read it as "my quit was accepted" and left too — the mass disconnect.
	m.broadcastData(packet.CmdPlayerQuit, message.PlayerAlternateQuit(s.entityID),
		fmt.Sprintf("cmd=102 PLAYER_QUIT ent=%#x %q -> others", s.entityID, s.player.Name))
	m.emitDeath(s.entityID, 0, 1, 0, 0, 2, m.deathPos(s.entityID), false, false)
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
	// Re-point anyone who was spectating the player who just left (the quitter is already out of
	// m.players, so spectateFor picks a still-present live teammate). cmd 149 keeps their view + our
	// OB_COUNT/relays in sync instead of stuck on the removed entity.
	m.repointSpectators(s.entityID, 0)
	log.Printf("[mm-udp] roster- slot=%d ent=%#x (roster=%d) %v", s.slot, s.entityID, len(m.players), s.remote)
}

// admitFirst runs the join handshake for the FIRST player of a fresh match: pick the arena/spawn,
// seed the round + enemy bot, answer the join (cmd 100 + cmd 101 self + cmd 101 bot + cmd 130),
// draw the zone, and start the match loop. m.arena/round/bot are the match's — set once here. Later
// players (Step 5b) are admitted onto the already-running loop instead of running this.
func (m *Match) admitFirst(s *session) {
	m.loadMatchSettings(s.player.MatchID) // custom-room round count / economy (or defaults) — before the GRI/round setup
	m.setupMatch(s)
	s.sendDataLog(packet.CmdJoinMatchRes, message.JoinMatchRes(0, m.settings.roomSetting, m.settings.roomSetting2), "cmd=100 LGIGCGIDOKP result=0")
	s.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(s.player, m.serverTick()),
		fmt.Sprintf("cmd=101 GKBDLJFGGMI acc=%d ent=%d %q", s.player.AccountID, s.player.EntityID, s.player.Name))

	if m.botEntity != 0 {
		s.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(m.bot, m.serverTick()),
			fmt.Sprintf("cmd=101 BOT acc=%d ent=%#x %q pos=(%.1f,%.1f,%.1f)",
				m.bot.AccountID, m.bot.EntityID, m.bot.Name, m.bot.SpawnPos.X, m.bot.SpawnPos.Y, m.bot.SpawnPos.Z))
	}

	s.sendDataLog(packet.CmdJoinMatchFinished, message.JoinMatchFinished(), "cmd=130 JoinMatchFinished")
	s.sendDataLog(packet.CmdSyncRegionActivities, message.SyncRegionActivitiesDefault(),
		"cmd=176 SYNC_REGION_ACTIVITIES (DEFAULT) — trigger dynamic-prefab visibility (jump pads/boxes)")
	s.sendData(packet.CmdInGameChat, message.SendInGameMessages([]string{"Apenas testando umas coisas aqui"}, "System"))
	s.broadcastZone()      // draw the safe zone at the NEW city now that the player joined
	m.broadcastZoneIndex() // name that city in the round-intro UI (cmd 457 per-player zone index)
	s.startCSMatch()
}

// broadcastZoneIndex pushes each player's per-round zone index (cmd 457) so the round-intro UI names the
// SAME city the cmd-145 teleport drops them in. The client keys m_CSSOPlayerZoneIndexDict by the player
// UID and the round-start UI resolves the LOCAL player's zone via GDADEBKBCOI -> the {MapID}_GameZoneInfo
// CSV name (protocol/gamezoneinfodec.txt, ZoneID 0-11). The dict defaults to 99 (a miss) until this is
// sent, so this is the actual city-name lever (the ShowCase cmd-294 path did nothing). See cs-round-city.
func (m *Match) broadcastZoneIndex() {
	zone := byte(m.arena.ZoneID)
	entries := make([]message.ZoneEntry, 0, len(m.players))
	for _, p := range m.players {
		entries = append(entries, message.ZoneEntry{UID: p.entityID, Packed: message.PackZone(zone, p.faction)})
	}
	m.broadcastData(packet.CmdZoneIndex, message.CSZoneIndex(entries),
		fmt.Sprintf("cmd=457 zone-index city=%q zone=%d", m.arena.City, m.arena.ZoneID))
}

// setupMatch initialises a fresh match's world for its first player: pick the arena/spawn, add the
// player to the roster, seed round 1, and create the enemy bot (consuming slot 2 so later humans get
// slot 3+). Shared by admitFirst and the reconnect resume (which skips the join packets).
func (m *Match) setupMatch(s *session) {
	if m.arenasUsed == nil {
		m.arenasUsed = make(map[csArena]bool)
	}

	m.arena = choosePlayerSpawn(&s.player, m.arenasUsed)
	m.arenasUsed[m.arena] = true
	m.addPlayer(s)
	// choosePlayerSpawn placed the first player at the LEFT fence (localFaction=0); once addPlayer has
	// assigned the real faction (a room can put a team-2 player in first), re-anchor to the correct
	// fence — matching admitLater. Skipped under the fixed-spawn test override (empty arena).
	if cfg.spawn == nil {
		s.player.SpawnPos, s.player.SpawnFace = m.arena.spawnFor(s.faction)
		s.playerPos = s.player.SpawnPos
	}
	m.round = 1
	m.allocSlot(0) // reserve slot 2 for the filler bot (return discarded; only bumps nextSlot)
	m.botEntity = 0
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
	m.hp[s.entityID] = m.settings.maxHP
	m.life[s.entityID] = lifeAlive

	// The bot was only a 1-human filler; a 2nd human retires it so the humans face each other (§2).
	if m.botEntity != 0 {
		m.retireBot()
	}

	s.sendDataLog(packet.CmdJoinMatchRes, message.JoinMatchRes(0, m.settings.roomSetting, m.settings.roomSetting2), "cmd=100 join res (2nd+ player)")
	s.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(s.player, m.serverTick()), // the joiner's OWN entity FIRST (client adopts the first as its local player)
		fmt.Sprintf("cmd=101 self ent=%d %q", s.player.EntityID, s.player.Name))
	for _, p := range m.players {
		if p != s { // then the already-present humans
			s.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(p.player, m.serverTick()),
				fmt.Sprintf("cmd=101 other ent=%d %q", p.player.EntityID, p.player.Name))
		}
	}
	if m.botEntity != 0 { // only if the bot is still around (1-human match); retired once a 2nd human joins
		s.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(m.bot, m.serverTick()), fmt.Sprintf("cmd=101 BOT ent=%#x", m.bot.EntityID))
	}
	for _, p := range m.players { // the joiner learns each existing player's inventory (held weapon + skin + back-mounted loadout)
		if p != s {
			s.sendDataLog(packet.CmdSyncInventory, p.currentInventorySync(),
				fmt.Sprintf("cmd=174 existing ent=%#x inventory -> joiner", p.entityID))
			p.resendEquipTo(s)  // cmd 121 so the joiner back-mounts each existing player's equipped weapons
			p.resendAttachTo(s) // + mount their maxed attachments so the joiner sees full guns, not bare ones
		}
	}
	s.sendDataLog(packet.CmdJoinMatchFinished, message.JoinMatchFinished(), "cmd=130 JoinMatchFinished")
	s.sendDataLog(packet.CmdSyncRegionActivities, message.SyncRegionActivitiesDefault(),
		"cmd=176 SYNC_REGION_ACTIVITIES (DEFAULT) — trigger dynamic-prefab visibility (jump pads/boxes)")

	for _, p := range m.players { // every already-present human learns the joiner
		if p != s {
			p.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(s.player, m.serverTick()),
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
	coins := csStartingCoins(m.settings.baseCoins) // wallet seed = economy table[0] (default 500)
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
			m.stepFlags(now)   // despawn emote-planted battle flags past their 60s lifetime
			m.streamMovement() // Step 6: relay each human's latest cmd-1001 state to the others
			for _, p := range m.players {
				p.stepHeal(now)        // medkit heal-over-time, per player (run()-driven)
				p.stepEP(now)          // mushroom EP -> HP regen, per player
				p.stepHealChannel(now) // stop a stuck medkit cure animation on interrupt/timeout
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
		param = 0 // the brief Waiting intro shows no phase countdown
	}
	// Held Prepare (waiting) and phCancelled both use phaseParam() as the field-3 countdown deadline so the
	// client renders the REAL remaining seconds (param - CurrentServerTime): ~30s while waiting for players,
	// then ~10s while cancelling. The old param=65000 "hold the buy timer high" hack rendered as a ~1083-min
	// timer once match mode 6 made the phase-countdown HUD exist. phaseParam stays constant across a held
	// phase (fixed deadline), so it doesn't re-fire the client's phase-change handler each tick.
	state, stateDeadline := m.matchState()
	s.sendVar(packet.CmdPRISync, m.priPayload(), 1)
	s.sendVar(packet.CmdGRISync, message.CSGRIInit(m.settings.maxRound, uint8(m.round-1)), 1) // fields 1,2 (round config)
	s.sendVar(packet.CmdGRISync, message.CSGRIMatchState(state, stateDeadline), 1)            // field 5 (waiting/cancel overlay)
	s.sendVar(packet.CmdGRISync, message.CSGRIPhase(phase, param), 1)
	point := 0
	if m.teamScore[0] == m.settings.roundsToWin-1 || m.teamScore[1] == m.settings.roundsToWin-1 {
		point = 1 // next round is the decider
	}
	s.sendVar(packet.CmdGRISync, message.CSGRIMatchPoint(uint8(point)), 1)
}

// streamClock streams cmd 1000 (server clock) to every human in the roster so the CS countdowns
// tick. It is SendUnreliable + plaintext (no per-connection seq), so the same packet fans to all.
// Replaces serverTimeLoop.
func (m *Match) streamClock() {
	// The client's CS server clock is CurrentServerTime = FixedDeltaTime × (localTick − anchor)
	// (KEPDHPAAHGP::OLAPLOIAMKM @0x1b38390), and cmd 1000 re-anchors it to THIS tick. So the tick MUST
	// use the same scale as the cmd-101 anchor (serverTick = seconds / clientFixedDelta ≈ ×30.303) — a
	// bare ×30 makes the client clock run ~1% slow, so its interpolated SafeZone circle lags behind our
	// real-time damage radius and the player takes damage while still visibly inside the zone.
	tick := uint32(time.Since(m.matchStart).Seconds() / clientFixedDelta)
	pkt := &packet.Packet{SendOption: packet.SendUnreliable, Cmd: packet.CmdSyncServerTime, Flags: 0, Payload: message.SyncServerTime(tick)}
	for _, p := range m.players {
		if p.out != nil {
			p.out.send(pkt, "")
		}
	}
}

// serverTick is the match-tick the LOCAL player's cmd 101 (field CEDJCPLOLNE) anchors the client's
// server clock to: the client sets CurrentServerTime = clientFixedDelta × serverTick at join, then
// advances it at 1/s, so this makes CurrentServerTime read match-seconds — the same units phaseParam
// and the field-5 deadline use, so every seconds countdown renders. Before the match loop sets
// matchStart (the very first join, admitFirst) it is 0 = match start. Recomputing it on EVERY cmd 101
// keeps the clock continuous across resends (a constant would snap the clock back each time). This is
// the fix for the inflated clock (was CEDJCPLOLNE = EntityID ≈ 16.7M → CurrentServerTime ≈ 553,000 s).
func (m *Match) serverTick() uint32 {
	if m.matchStart.IsZero() {
		return 0
	}
	return uint32(time.Since(m.matchStart).Seconds() / clientFixedDelta)
}

// streamMovement relays each human's latest cmd-1001 state to EVERY player INCLUDING the sender (Step
// 6): one batch per sender. The batch's skip-own byte is 1, so the LIVE sender DROPS its own entry
// (its pawn stays 100% input-driven — no rubber-band), while every OTHER client applies it. Sending it
// to the sender too is what puts the recorder's OWN motion into its replay recording: the replay apply
// forces skip-own OFF (IsReplayState), so the recorder's own entry moves its pawn on playback. The bot
// is static and isn't relayed — the client keeps it at its cmd-101 spawn. The seq is the subtle part.
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
			if recv.out != nil { // incl. the sender — skip-own drops its own entry live; the recording keeps it for replay
				recv.out.send(pkt, "")
			}
		}
	}
}

// reseed tuning: re-emit the roster up to reseedMax times, >= reseedEvery apart, so the burst reliably
// lands INSIDE the replay recording window regardless of how long the client's scene load takes.
const (
	reseedMax   = 12
	reseedEvery = 1 * time.Second
)

// reseedJoinBurst re-emits the cmd-101 roster into the replay recording window. The ORIGINAL join burst
// (admitFirst, in reply to cmd 440) is pulled + dispatched during the LOADING screen, BEFORE the client
// arms its recorder at OnSceneLoaded, so it never lands in the recording — a client replay then spawns
// no pawn and hangs forever on the loading mask (the world-streamer centers tile-loading on the local
// pawn). We can't observe OnSceneLoaded server-side, and the client may emit its first cmd 1001 (the
// trigger) DURING loading (its pawn already exists), which would be BEFORE recording arms. So we don't
// reseed just once: we re-emit up to reseedMax times, throttled to reseedEvery, spanning the scene-load
// boundary — the copies that arrive after OnSceneLoaded are recorded (the earlier ones are harmless).
//
// Each pass re-emits cmd 101 (spawn the pawns) THEN cmd 230 (seed the replay observer). cmd 101 alone
// is NOT enough to dismiss: a replay has no local player (Player::IsLocalPlayer is false in replay
// state), so the streamer-finished dismiss gate never trips. cmd 230 (EObserverType_Replay) dismisses
// via the unconditional OnAddObserver -> CloseMask path AND gives the camera/streamer a focus.
//
// Every packet here is a NO-OP on the LIVE client: the roster-add (NFJPHMKKEBF::LONDNBHBPDO) early-
// returns on an already-present entity (no re-spawn/snap, local pawn too), and cmd 230 type 2 returns
// immediately when not in replay state. NEVER cmd 100/145/900 (those reposition/re-init and DO glitch
// a live pawn). In the replay the first recorded pass spawns the pawn + seeds the observer + dismisses
// loading; the rest no-op. Called on each cmd 1001 (handleClientPos).
func (m *Match) reseedJoinBurst() {
	if m.reseedCount >= reseedMax {
		return
	}
	now := time.Now()
	if m.reseedCount > 0 && now.Sub(m.lastReseed) < reseedEvery {
		return
	}
	m.lastReseed = now
	m.reseedCount++
	tick := m.serverTick()
	for _, recv := range m.players {
		if recv.out == nil {
			continue
		}
		recv.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(recv.player, tick), "cmd=101 replay-reseed self")
		for _, other := range m.players {
			if other != recv {
				recv.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(other.player, tick), "cmd=101 replay-reseed roster")
			}
		}
		if m.botEntity != 0 {
			recv.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(m.bot, tick), "cmd=101 replay-reseed bot")
		}
		// AFTER the pawn(s) are spawned in the recording: seed the replay observer (cmd 230, type
		// EObserverType_Replay). This is a LIVE no-op but is what makes the REPLAY dismiss its loading
		// mask (OnAddObserver -> CloseMask, unconditional) + gives the camera/streamer a focus — cmd 101
		// alone can't (a replay has no local player, so the streamer-finished dismiss gate never trips).
		recv.sendDataLog(packet.CmdObserverJoin, message.ObserverJoin(recv.player.AccountID, recv.entityID),
			"cmd=230 replay observer-seed (EObserverType_Replay)")

		// NOTE: no loadout re-sync here. The starter loadout's own cmd 174 (from giveLoadout at the buy
		// phase) is broadcast post-scene and IS recorded WITH its equipment list, so the loadout data is
		// already in the recording; re-sending it here only duplicated items (cmd 174 is additive) or,
		// on the first pass, captured an empty loadout (the reseed fires ~3s before giveLoadout runs).
	}

	// Replay weapon-wheel kick (ONCE, after the loadout is recorded). The observer HUD's slot buttons only
	// re-activate when the observed pawn's ON-HAND changes (OBSERVER_INVENTORY_ITEM_ON_HAND_CHANGED sets
	// m_weaponChanged -> RefreshUIByWeaponOnHand); cmd 174 fills the slot data but never fires it, so a
	// replay shows only the held weapon + fists until the recorder's first weapon switch. m.started means
	// giveLoadout already ran + its cmd 174 is recorded, so this cmd-108 on-hand toggle lands AFTER the
	// loadout in the recording and forces the wheel to repaint the equipped slots. See kickObserverWheel.
	if !m.wheelKicked && m.started {
		m.wheelKicked = true
		for _, recv := range m.players {
			if recv.out != nil {
				recv.kickObserverWheel()
			}
		}
	}

	log.Printf("[mm-udp] replay-reseed #%d/%d: re-emitted cmd-101 roster to %d client(s) for the recording", m.reseedCount, reseedMax, len(m.players))
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

// enterPrepare opens the buy phase. On round 1 with too few players it HOLDS Prepare (the player is
// unfrozen — mask off, shop open, can move at the gate) and shows the "Waiting for players" overlay (GRI
// field 5) counting down, so the client isn't stuck in the frozen Waiting phase; advancePhase then starts
// the buy phase when players arrive (startBuyPhase) or cancels at the deadline. With enough players, or on
// any later round, it goes straight to the normal buy phase (+ lock). holdPrepare is the dev shop-hold mode.
func (m *Match) enterPrepare(now time.Time) {
	m.pull = nil
	if cfg.holdPrepare { // dev shop-testing: never leave Prepare
		m.started = true
		m.enter(now, phPrepare, holdDur)
		return
	}
	if !m.started && !m.teamsReady() {
		m.enter(now, phPrepare, waitPlayersDur) // hold Prepare + show the waiting overlay (field 5 = HalfwayJoin)
		return
	}
	m.startBuyPhase(now) // enough players (or a later round): normal buy phase + lock the roster
}

// startFight begins the Fight phase (ends on the enemy/player-dead condition, not the deadline)
// and starts the safe zone. holdDur only feeds phaseParam a stable countdown; advancePhase ends
// Fight via the condition check, which runs before the deadline check.
func (m *Match) startFight(now time.Time) {
	m.enter(now, phFight, holdDur)
	m.startZone()
}

// teamsReady reports whether the roster is balanced enough to start: both teams have at least one human AND
// equal counts. allocSlot alternates joiners across the two teams, so 2 players => 1v1 => ready; a lone
// player sits at 1v0 and keeps waiting.
func (m *Match) teamsReady() bool {
	var t1, t2 int
	for _, p := range m.players {
		switch p.team {
		case 1:
			t1++
		case 2:
			t2++
		}
	}
	return t1 > 0 && t2 > 0 && t1 == t2
}

// matchState returns the GRI field-5 (EMatchDrawState) value + its lo16 deadline second for the current
// phase: the "Waiting for players" overlay while gathering players, the "Match cancelled" (CutShort) overlay
// while cancelling, else NormalStart (no overlay). The waiting countdown reuses the same match-clock deadline
// second the phase-3 countdown uses (phaseParam), so client and server agree.
func (m *Match) matchState() (uint16, uint16) {
	switch m.phase {
	case phPrepare:
		if !m.started { // Prepare held for players -> "Waiting for players" overlay (state 1) + its deadline second
			return message.CSDrawHalfwayJoin, m.phaseParam()
		}
	case phCancelled:
		return message.CSDrawDraw, 0 // "Match cancelled" in-match overlay (state 2; the client fixes the 10s CutShort countdown)
	}
	// Normal play: actively CLEAR any overlay with Resume/CancelDraw (state 4 -> dispatch Hide), NOT
	// NormalStart (0). The field-5 OnRep is change-only, and state 0 does not dispatch a hide, so a lone
	// 1->0 transition would leave the "Waiting for players" overlay stuck on screen when the match starts
	// early (teams balanced before the deadline). State 4 guarantees the overlay is hidden the moment the
	// match proceeds; on a match that never waited it fires one harmless no-op Hide.
	return message.CSDrawCancelDraw, 0
}

// startBuyPhase LOCKS the match (canAdmit false -> latecomers get a fresh match) and opens the normal
// buy-phase countdown. Called the moment enough players arrive during the waiting hold, when a bot fills in,
// and for every round after the first. The shop + starting loadout were already sent at the phSetupWait edge
// (and to joiners at admitLater), so this only sets the deadline + lock; the field-5 waiting overlay clears
// on the next stream now that m.started is set (matchState -> NormalStart).
func (m *Match) startBuyPhase(now time.Time) {
	first := !m.started
	m.started = true
	m.enter(now, phPrepare, cfg.buyPhase)
	if first {
		log.Printf("[mm-udp] match started (roster=%d bot=%#x) -> buy phase; locked to new joins", len(m.players), m.botEntity)
	}
}

// enterCancel begins the pre-match cancel: mark the match ended (no rejoin) and hold the "Match cancelled"
// overlay (field 5 CutShort) for cancelHoldDur, after which advancePhase sends ReturnToLobby and reaps it.
func (m *Match) enterCancel(now time.Time) {
	m.ended = true
	m.enter(now, phCancelled, cancelHoldDur)
	log.Printf("[mm-udp] no opponents found in %s -> Match cancelled (return to lobby in %s)", waitPlayersDur, cancelHoldDur)
}

// spawnFallbackBot fills the enemy team with the bring-up bot when MATCH_BOT is set and the wait expired with
// no real opponent, so a solo player can still test a match. It re-enables the bot the way admitFirst used
// to: create its identity (near the primary player, facing them), seed HP/life, broadcast its cmd 101 join,
// and rebind so its PRI replicates. See retireBot for the inverse.
func (m *Match) spawnFallbackBot() {
	m.botEntity = botEntityID
	m.bot = botPlayer(m.s.player, m.botEntity)
	m.hp[m.botEntity] = m.settings.maxHP
	m.life[m.botEntity] = lifeAlive
	m.broadcastData(packet.CmdPlayerJoin, message.PlayerJoin(m.bot, m.serverTick()),
		fmt.Sprintf("cmd=101 BOT fallback ent=%#x (MATCH_BOT: no players found)", m.botEntity))
	m.bindAll()
	log.Printf("[mm-udp] MATCH_BOT: filled the opponent with a bot ent=%#x", m.botEntity)
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
	if m.phase == phPrepare && !m.started && m.teamsReady() { // players arrived during the waiting hold -> start the buy phase
		m.startBuyPhase(now)
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
		m.enter(now, phSetupWait, setupWaitDur) // brief Waiting intro before the shop opens
	case phSetupWait:
		s.sendCSShop()
		s.sendStartingLoadout()
		m.enterPrepare(now) // -> hold Prepare + waiting overlay (too few players), or the normal buy phase
	case phCancelled: // the "Match cancelled" hold ended -> return everyone to the lobby + reap the match
		s.sendVar(packet.CmdGRISync, message.CSGRIMatchState(message.CSDrawMatchEnd, 0), 1)
		if !m.tearingDown {
			m.tearingDown = true
			close(m.done)
		}
	case phPrepare:
		if m.started {
			m.startFight(now) // normal buy phase ended -> Fight
		} else if cfg.matchBot { // the waiting hold expired with too few players -> bot fill (MATCH_BOT dev) ...
			m.spawnFallbackBot()
			m.startBuyPhase(now)
		} else {
			m.enterCancel(now) // ... or cancel the match (Match cancelled -> lobby)
		}
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
		s.broadcastZone()      // draw the NEW city's safe zone now the player teleported
		m.broadcastZoneIndex() // name the NEW city in the round-intro UI (cmd 457)
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
	// Update team loss streaks (winner resets, loser increments) BEFORE computing the loss ladder.
	winIdx := int(winnerTeam) - 1
	m.lossStreak[winIdx] = 0
	m.lossStreak[1-winIdx]++
	// Per-player round coins — the REAL CS economy (economy.go), replacing the old flat formula:
	// running balance += base[round] + a win/loss event (loss stacks with the team's streak) +
	// per-kill. base[round] holds 3000 from round 7 on. (First-blood bonus: TODO, not tracked yet.)
	base := csRoundIncome(m.round, m.settings.baseCoins)
	ev := m.settings.events // event bonuses: economy.go defaults, or the advanced room's overrides
	for _, p := range m.players {
		aw := base + ev.perKill*p.roundKills.Swap(0)
		if p.team == winnerTeam {
			aw += ev.winRound
		} else {
			aw += ev.lossBonus(m.lossStreak[p.team-1])
		}
		if p.entityID == m.firstBlood { // the human who drew first blood this round
			aw += ev.firstBlood
		}
		p.coins += aw
		p.award = aw
	}
	m.firstBlood = 0 // reset for the next round (endFight is the once-per-round funnel that resets roundKills too)
	log.Printf("[mm-udp] ROUND %d won by team %d (score %d-%d)", m.round, winnerTeam, m.teamScore[0], m.teamScore[1])

	if m.teamScore[0] >= m.settings.roundsToWin || m.teamScore[1] >= m.settings.roundsToWin || m.round >= int(m.settings.maxRound) {
		m.matchWon = localWon
		m.winnerTeam = winnerTeam                    // the per-player cmd 103 rank reads this so each side gets its OWN win/lose
		m.enter(now, phMatchEndPause, matchEndPause) // pause on the final kill, then cmd 103
		return
	}
	s.roundResult(winnerTeam) // cmd 409 round result (non-deciding rounds only)
	m.revive = !localWon
	// Transition setup (was roundTransition step 1): pick the next arena + spawns + a fresh bot. nextArena
	// avoids any city already played this match (the old pickArena() picked purely at random, so rounds
	// could repeat a city).
	m.arena = m.nextArena()
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
		p.sendMushroomCount()           // cmd 533: reset the shop's mushroom "N/2" label for the new round (m_PurchaseCnt persists client-side otherwise)
	}
	s.clearWalls()    // gloo walls are per-round world state
	m.clearAllFlags() // emote-planted battle flags are per-round world state too (never auto-remove client-side)
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
