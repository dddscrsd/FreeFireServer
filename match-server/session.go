package main

import (
	"log"
	"net"
	"sync/atomic"
	"time"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// csWeapon tracks one weapon currently in the loadout so a round restart can refill its
// magazine (Runtime) and reserve ammo by RE-SENDING THE SAME uniques. The client's cmd
// 174 handler upserts inventory by Unique (SyncInventoryInfo -> OJKKGKBGKMJ: an existing
// Unique updates in place — for a weapon it sets the loaded clip = InvItem.Runtime via
// KANLCBHFONB::NKJJELOAGGL), so reusing the uniques resets ammo without duplicating.
type csWeapon struct {
	slot   byte
	data   uint32 // weapon DataID
	unique uint32 // weapon InvItem Unique
	ammo   uint32 // ammo DataID (the reserve itself is separate 30-round stacks tracked in clientUIDs)
}

// session is one client's connection state (keyed by remote UDP address). Once the match loop
// starts (startCSMatch), run() is the SOLE owner of the match state below: gameplay handlers run
// on run()'s goroutine via the mailbox, so none of it needs a lock (Step 3b). Before the match
// starts, the join handlers mutate it directly on the receive goroutine — run() isn't up yet, so
// there is no contention.
type session struct {
	conn   *net.UDPConn
	remote *net.UDPAddr
	key    []byte
	out    *Writer // this connection's outbound path — owns seq/order + every send (see writer.go)

	joined      bool
	syncStarted bool        // match loop already running (guard); once true, dispatch routes handlers to the mailbox
	match       *Match      // the match this player belongs to (created with the session, Step 4); its run() owns the shared world + the mailbox
	stopped     atomic.Bool // set on player quit (cmd 191); the match loop exits when true
	player      joinPlayer  // resolved from the prepare_token (cmd 439/440)

	// Allocated per-participant ids (Step 4b, via Match.allocSlot): entity id = team<<24 | slot,
	// PRI RepID, CS faction, team (the entity hibyte), and roster slot. For the first human these
	// equal the playerEntityID / playerRepID / localFaction constants the game logic still reads;
	// a later step swaps those constants for these per-session ids so a second player has its own.
	entityID uint32
	repID    uint32
	faction  byte
	team     byte
	slot     byte

	// prepare_token reassembly: a large token (many cosmetics) is split across cmd 439
	// (JOIN_MATCH_PREPARE, [u32 chunkLen][chunk], the bigger half) and cmd 440
	// (JOIN_MATCH_POST, the tail). prep439 buffers the 439 chunk; handleJoinMatchPost
	// appends the 440 tail to reassemble the full JWT.
	prep439 []byte

	// roundKills is this player's kills this round (reset each round; drives the coin award).
	// The shared round / score / clock / teleport-seq / arena / bot now live on the Match (4a).
	roundKills atomic.Uint32

	// Contra Squad economy/loadout state, owned by the match loop (run()). The buy-phase money
	// and the current equipment slot map so purchases (cmd 408) can deduct and add items.
	coins      uint32              // buy-phase money
	award      uint32              // coins awarded for the round (kills + win bonus) — added to coins at the next buy phase
	uidCounter uint32              // allocates unique item instance ids — MONOTONIC (never reset), so a new item can't collide with a stale one the client hasn't dropped yet
	equipment  []message.Equipment // full loadout slot map (re-sent whole each sync)
	weapons    map[byte]csWeapon   // current loadout weapons keyed by slot (2-primary placement + round-restart ammo refill)
	clientUIDs map[uint32]lootItem // uid -> item the client holds; source of truth for the cmd-327 respawn clear AND for dropping ANY item (cmd 112) by unique (consumables/throwables have no weapon slot)
	itemOnHand uint32              // unique of the currently held item

	// playerPos is this player's last-reported world position (cmd 1001), used by the SafeZone
	// out-of-zone check. (The per-entity HP map is shared world state on the Match now — 4a; it
	// splits to per-player in 4c.)
	playerPos message.Vec3

	// moveBody is this player's latest cmd-1001 state body (upstream [4:41] — pos+dirs+velocity+
	// state+aim+platform), re-framed verbatim into the movement relay batch (Step 6).
	moveBody []byte

	// moveBase / moveBaseTick anchor the relayed movement seq. The client's per-pawn seq gate
	// (PlayerNetwork::PushPlayerSyncedStateData: KINJCKMOGIM < seq) needs a LARGE seq — the pawn's
	// stored seq inits ~1.78e9 — but the pawn then interpolates over (newSeq-oldSeq) FRAMES with no
	// clamp for remotes, so the per-update DELTA must stay small or the pawn coasts. So the FIRST
	// upstream send-seq (captured in handleClientPos) becomes the large base, advanced only by the
	// 30Hz match-tick delta — the interpolation window stays ≈ the relay interval.
	moveBase     uint32
	moveBaseTick int32

	// firing is true while this player holds the trigger (cmd 104 actionType=2 -> cmd 105); streamed
	// as PRI field 21 START_FIRE_STATE so remote clients render its muzzle flash / tracer.
	firing bool

	// sighting is true while this player is aiming down sights / scoped (cmd 137 -> cmd 138); streamed
	// as PRI field 12 IS_SIGHTING so remote clients strike the scoped/aiming pose. See fire.go.
	sighting bool

	// Medkit heal-over-time (run()-driven): cmd 113 arms it, stepHeal applies the accrued HP
	// each tick until full HP / death / all steps done. See medkit.go.
	healActive  bool
	healStart   time.Time
	healApplied int

	// Shop-mushroom EP (energy): buying a mushroom grants EP; stepEP drains it back into HP over
	// time. mushrooms counts buys this round (capped at mushroomsPerRound); both reset each round.
	// See ep.go.
	ep        uint16
	mushrooms int
	epLast    time.Time

	// Armor durability (helmet + vest): the client applies the damage reduction to its own HP; the
	// server only tracks the wear for the PRI armor bars (fields 2/3). Set on purchase, dropped on
	// hits, broken (piece cleared) at 0. See durability.go.
	vestData, helmetData uint32
	vestDur, helmetDur   uint16
}

// write queues one packet to this session's remote through its Writer, which owns the
// seq/order counters and the socket; senders never take a lock, they just enqueue.
func (s *session) write(p *packet.Packet) {
	s.out.send(p, "")
}

// kick sends a SendKick (so=3) packet, which the client's transport treats as "kicked
// by server" and returns to the lobby — the cmd/payload are not significant, so=3 is
// the kick signal. Sent to every client on server shutdown (see kickAllAndExit).
func (s *session) kick() {
	s.out.send(&packet.Packet{SendOption: packet.SendKick, Cmd: packet.CmdMatchEnd, Flags: 0}, "")
}

// ack queues the unreliable cmd=2 ACK for a received reliable seq (pre-encoded, sent verbatim).
func (s *session) ack(seq uint16) {
	wire, err := packet.BuildAck(seq, 0, s.key)
	if err != nil {
		log.Printf("[mm-udp] ack err: %v", err)
		return
	}
	s.out.sendRaw(wire)
}

// sendHelloRes queues the S2C_Hello_Res (send_option=1); the Writer assigns the next seq (the
// hello reply is not part of the ordered data stream, so its order stays 0).
func (s *session) sendHelloRes(payload []byte) {
	s.out.send(&packet.Packet{SendOption: packet.SendHello, Cmd: packet.CmdHello, Flags: 0, Payload: payload}, "")
}

// sendData queues a reliable, encrypted send_option=2 application packet; the Writer assigns
// the next seq + data order id.
func (s *session) sendData(cmd uint16, payload []byte) {
	s.out.send(&packet.Packet{SendOption: packet.SendReliable, Cmd: cmd, Flags: packet.FlagEncrypted, Payload: payload}, "")
}

// sendDataLog queues a reliable data packet; the Writer logs it once sent, with the assigned
// seq/order. `what` is a short label (e.g. "cmd=100 LGIGCGIDOKP result=0").
func (s *session) sendDataLog(cmd uint16, payload []byte, what string) {
	s.out.send(&packet.Packet{SendOption: packet.SendReliable, Cmd: cmd, Flags: packet.FlagEncrypted, Payload: payload}, what)
}

// sendVar frames an unreliable VAR replication packet (send_option=4 — the channel the client
// consumes cmd 900 PRI / 901 GRI on) ONCE, then fans it to every human in the match roster
// `repeat` times to survive drops. Encode-once is safe because VAR framing carries no
// per-connection state, so the bytes are identical for all players — this is the multiplayer
// broadcast primitive. With a single human the roster is just this session (Step 4b/R2).
func (s *session) sendVar(cmd uint16, payload []byte, repeat int) {
	if repeat < 1 {
		repeat = 1
	}
	wire := frameVar(cmd, payload, s.key)
	outs := s.match.rosterWriters()
	for i := 0; i < repeat; i++ {
		broadcast(wire, outs)
	}
}
