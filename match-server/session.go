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
	slot    byte
	data    uint32 // weapon DataID
	unique  uint32 // weapon InvItem Unique
	ammo    uint32 // ammo DataID
	ammoUID uint32 // ammo InvItem Unique
}

// session is one client's connection state (keyed by remote UDP address).
type session struct {
	conn   *net.UDPConn
	remote *net.UDPAddr
	key    []byte

	mu            sync.Mutex
	sendSeq       uint16 // global reliable sequence counter
	sendDataOrder uint16 // order counter for send_option=2 data packets

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
	uidCounter uint32              // allocates unique item instance ids
	equipment  []message.Equipment // full loadout slot map (re-sent whole each sync)
	weapons    map[byte]csWeapon   // current loadout weapons keyed by slot (2-primary placement + round-restart ammo refill)
	itemOnHand uint32              // unique of the currently held item

	// Per-entity current HP (entity game id -> HP), guarded by hpMu. Damage reports
	// (cmd 106) decrement it; the PRI stream replicates it so the client sees kills.
	// playerPos is the local player's last-reported world position (cmd 1001), used by
	// the SafeZone to decide who is outside the circle. Also guarded by hpMu.
	hpMu      sync.Mutex
	hp        map[uint32]uint16
	playerPos message.Vec3
}

// write encodes and sends one packet to this session's remote.
func (s *session) write(p *packet.Packet) {
	wire, err := p.Encode(s.key)
	if err != nil {
		log.Printf("[mm-udp] encode cmd=%d err: %v", p.Cmd, err)
		return
	}
	if _, err := s.conn.WriteToUDP(wire, s.remote); err != nil {
		log.Printf("[mm-udp] write err: %v", err)
	}
}

// kick sends a SendKick (so=3) packet, which the client's transport treats as "kicked
// by server" and returns to the lobby — the cmd/payload are not significant, so=3 is
// the kick signal. Sent to every client on server shutdown (see kickAllAndExit).
func (s *session) kick() {
	s.write(&packet.Packet{SendOption: packet.SendKick, Cmd: packet.CmdMatchEnd, Flags: 0})
}

// ack sends the unreliable cmd=2 ACK for a received reliable seq.
func (s *session) ack(seq uint16) {
	wire, err := packet.BuildAck(seq, 0, s.key)
	if err != nil {
		log.Printf("[mm-udp] ack err: %v", err)
		return
	}
	s.conn.WriteToUDP(wire, s.remote)
}

// sendHelloRes sends the S2C_Hello_Res (send_option=1) using the next seq; the hello
// reply is not part of the ordered data stream, so order stays 0.
func (s *session) sendHelloRes(payload []byte) (seq uint16) {
	s.mu.Lock()
	seq = s.sendSeq
	s.sendSeq++
	s.mu.Unlock()
	s.write(&packet.Packet{SendOption: packet.SendHello, Cmd: packet.CmdHello, SeqID: seq, OrderID: 0, Flags: 0, Payload: payload})
	return
}

// sendData sends a reliable, encrypted send_option=2 application packet, assigning
// the next seq and data order id.
func (s *session) sendData(cmd uint16, payload []byte) (seq, order uint16) {
	s.mu.Lock()
	seq = s.sendSeq
	s.sendSeq++
	order = s.sendDataOrder
	s.sendDataOrder++
	s.mu.Unlock()
	s.write(&packet.Packet{SendOption: packet.SendReliable, Cmd: cmd, SeqID: seq, OrderID: order, Flags: packet.FlagEncrypted, Payload: payload})
	return
}

// sendDataLog sends a reliable data packet and logs it. `what` is a short label
// (e.g. "cmd=100 LGIGCGIDOKP result=0") — this replaces the repeated
// send-then-log-Printf pattern at the call sites.
func (s *session) sendDataLog(cmd uint16, payload []byte, what string) {
	seq, order := s.sendData(cmd, payload)
	log.Printf("[mm-udp] -> %s (seq=%d order=%d %dB) %v", what, seq, order, len(payload), s.remote)
}

// sendVar sends an unreliable VAR replication packet (send_option=4), the channel
// the client consumes cmd 900 (PRI) / 901 (GRI) on. VAR packets are plaintext
// (flags=0) and unreliable (no seq/order), so they are resent `repeat` times / on a
// loop to survive drops.
func (s *session) sendVar(cmd uint16, payload []byte, repeat int) {
	if repeat < 1 {
		repeat = 1
	}
	for i := 0; i < repeat; i++ {
		s.write(&packet.Packet{SendOption: packet.SendVar, Cmd: cmd, Flags: 0, Payload: payload})
	}
}
