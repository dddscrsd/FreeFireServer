package main

import (
	"log"
	"net"
	"sync"
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

// session is one client's connection state (keyed by remote UDP address).
type session struct {
	conn   *net.UDPConn
	remote *net.UDPAddr
	key    []byte
	out    *Writer // this connection's outbound path — owns seq/order + every send (see writer.go)

	joined      bool
	syncStarted bool        // csSyncLoop already running for this session (guard)
	stopped     atomic.Bool // set on player quit (cmd 191); the CS sync loop exits when true
	player      joinPlayer  // resolved from the prepare_token (cmd 439/440)
	bot         joinPlayer  // the current-round enemy bot
	arena       csArena     // CS spawn city for the current round (re-picked each round)

	// prepare_token reassembly: a large token (many cosmetics) is split across cmd 439
	// (JOIN_MATCH_PREPARE, [u32 chunkLen][chunk], the bigger half) and cmd 440
	// (JOIN_MATCH_POST, the tail). prep439 buffers the 439 chunk; handleJoinMatchPost
	// appends the 440 tail to reassemble the full JWT.
	prep439 []byte

	// Round state. round is 1-based; botEntity is the (fixed) enemy bot entity id,
	// revived+repositioned each round rather than replaced; tpSeq sequences the
	// force-teleports used to reposition entities between rounds. matchStart marks the
	// CS clock origin used to compute phase-countdown deadlines (client shows
	// param - GameTime, so param = secondsSinceMatchStart + phaseSeconds).
	round      int
	botEntity  uint32
	teamScore  [2]uint8 // rounds won: [0]=local (faction 0), [1]=bot (faction 1)
	tpSeq      uint32
	matchStart time.Time
	roundKills atomic.Uint32 // local player's kills this round (reset each round; drives the coin award)

	// Contra Squad economy/loadout state, guarded by invMu. The buy-phase money and
	// the current equipment slot map so purchases (cmd 408) can deduct and add items.
	invMu      sync.Mutex
	coins      uint32              // buy-phase money
	award      uint32              // coins awarded for the round (kills + win bonus) — added to coins at the next buy phase
	uidCounter uint32              // allocates unique item instance ids — MONOTONIC (never reset), so a new item can't collide with a stale one the client hasn't dropped yet
	equipment  []message.Equipment // full loadout slot map (re-sent whole each sync)
	weapons    map[byte]csWeapon   // current loadout weapons keyed by slot (2-primary placement + round-restart ammo refill)
	clientUIDs map[uint32]lootItem // uid -> item the client holds; source of truth for the cmd-327 respawn clear AND for dropping ANY item (cmd 112) by unique (consumables/throwables have no weapon slot)
	itemOnHand uint32              // unique of the currently held item

	// Ground loot, guarded by invMu (same lock as the loadout it flows to/from, so a
	// drop↔loadout mutation stays atomic). containers holds the live ground pickup boxes
	// keyed by their wire ContainerObjectID; nextContainerID is the runtime id allocator.
	containers      map[uint16]*container // live ground boxes keyed by ContainerObjectID
	nextContainerID uint16                // runtime container-id allocator (runtimeContainerBase..Max)

	// Placed gloo (ice) walls, guarded by invMu (the SAME lock as clientUIDs, so the
	// PLACE path can read the gloo inventory count, deduct it, mutate the wall list and
	// compute the FIFO cap in one atomic step). walls is FIFO-ordered: walls[0] is the
	// oldest placed. wallSeq is a monotonic allocator (never reused) for wall entity ids
	// and per-wall PRI RepIDs. See gloo.go.
	walls   []*iceWall
	wallSeq uint32

	// Per-entity current HP (entity game id -> HP), guarded by hpMu. Damage reports
	// (cmd 106) decrement it; the PRI stream replicates it so the client sees kills.
	// playerPos is the local player's last-reported world position (cmd 1001), used by
	// the SafeZone to decide who is outside the circle. Also guarded by hpMu.
	hpMu      sync.Mutex
	hp        map[uint32]uint16
	playerPos message.Vec3
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

// sendVar queues `repeat` copies of an unreliable VAR replication packet (send_option=4), the
// channel the client consumes cmd 900 (PRI) / 901 (GRI) on. VAR packets are plaintext (flags=0)
// and carry no seq/order, so they are resent to survive drops.
func (s *session) sendVar(cmd uint16, payload []byte, repeat int) {
	if repeat < 1 {
		repeat = 1
	}
	for i := 0; i < repeat; i++ {
		s.out.send(&packet.Packet{SendOption: packet.SendVar, Cmd: cmd, Flags: 0, Payload: payload}, "")
	}
}
