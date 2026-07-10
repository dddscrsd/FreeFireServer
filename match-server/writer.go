package main

import (
	"encoding/binary"
	"log"
	"net"
	"time"

	"libmadoka/match-server/packet"
)

// RUDP retransmission tuning. A reliable (so=2) send is buffered until the client ACKs
// its seq; if the ACK doesn't arrive within rudpRTO it is resent (same bytes / seq, so the
// client de-dupes by seq), up to rudpMaxRetries. The client ACKs each reliable packet
// individually (cmd 2, payload [seq u16][ackBits u32]) — the server used to IGNORE those,
// so a dropped death/inventory/teleport was lost forever.
const (
	rudpRTO        = 250 * time.Millisecond
	rudpTick       = 50 * time.Millisecond
	rudpMaxRetries = 8
)

// Writer is one connection's outbound path. A single goroutine (run) owns the seq/order
// counters, the retransmit buffer, and every WriteToUDP for this remote, so senders never
// take a lock — they just enqueue. Framing in one place is also the fan-out primitive the
// multiplayer broadcast is built on: a match hands the same pre-framed VAR bytes to every
// player's Writer.
type Writer struct {
	conn *net.UDPConn
	addr *net.UDPAddr
	key  []byte
	ch   chan outItem
	acks chan uint16 // client ACKs (seq) routed here from the receive loop; drained by run()

	// Owned SOLELY by run(): the reliable sequence counter (shared by hello + reliable data),
	// the send_option=2 data order counter, and the un-ACKed reliable-send buffer. No mutex —
	// run() is the only mutator.
	seq     uint16
	order   uint16
	pending map[uint16]*pendingPkt
}

// pendingPkt is an un-ACKed reliable send held for retransmit: the fully-framed wire bytes
// (resent verbatim, keeping the same seq so the client de-dupes), the next resend deadline,
// and how many times it has been resent.
type pendingPkt struct {
	wire     []byte
	deadline time.Time
	retries  int
}

// outItem is one queued send: either a pre-encoded packet emitted verbatim (raw — e.g. an ack)
// or a logical packet the Writer frames with this connection's next seq/order.
type outItem struct {
	pkt   *packet.Packet
	raw   []byte
	label string
}

// newWriter starts a Writer and its run() goroutine for the given remote.
func newWriter(conn *net.UDPConn, addr *net.UDPAddr, key []byte) *Writer {
	w := &Writer{
		conn:    conn,
		addr:    addr,
		key:     key,
		ch:      make(chan outItem, 256),
		acks:    make(chan uint16, 256),
		pending: map[uint16]*pendingPkt{},
	}
	go w.run()
	return w
}

// run is the sole owner of seq/order, the socket, and the retransmit buffer for this remote.
// It frames + sends each queued item (FIFO), removes ACKed packets from the buffer, and resends
// un-ACKed reliable packets whose RTO elapsed — all on one goroutine, so no lock is needed.
func (w *Writer) run() {
	t := time.NewTicker(rudpTick)
	defer t.Stop()
	for {
		select {
		case it, ok := <-w.ch:
			if !ok {
				return
			}
			w.emit(it)
		case seq := <-w.acks:
			delete(w.pending, seq) // client confirmed this reliable seq — stop retransmitting it
		case now := <-t.C:
			w.retransmit(now)
		}
	}
}

// emit frames + sends one queued item, stamping seq/order for reliable packets and buffering
// ordered reliable data (so=2) for retransmit until the client ACKs its seq.
func (w *Writer) emit(it outItem) {
	if it.pkt == nil { // pre-encoded (ack): emit verbatim
		if it.raw != nil {
			if _, err := w.conn.WriteToUDP(it.raw, w.addr); err != nil {
				log.Printf("[mm-udp] write err: %v", err)
			}
		}
		return
	}
	if packet.IsReliable(it.pkt.Cmd, it.pkt.SendOption) { // hello (so=1) + reliable data (so=2)
		it.pkt.SeqID = w.seq
		w.seq++
		if it.pkt.SendOption == packet.SendReliable { // only ordered data carries an order id
			it.pkt.OrderID = w.order
			w.order++
		}
	}
	wire, err := it.pkt.Encode(w.key)
	if err != nil {
		log.Printf("[mm-udp] encode cmd=%d err: %v", it.pkt.Cmd, err)
		return
	}
	if _, err := w.conn.WriteToUDP(wire, w.addr); err != nil {
		log.Printf("[mm-udp] write err: %v", err)
	}
	// Buffer ordered reliable data (so=2) for retransmit. HELLO (so=1) is a one-shot handshake;
	// VAR/unreliable carry no seq, so they aren't retransmittable.
	if it.pkt.SendOption == packet.SendReliable {
		w.pending[it.pkt.SeqID] = &pendingPkt{wire: wire, deadline: time.Now().Add(rudpRTO)}
	}
	if it.label != "" {
		log.Printf("[mm-udp] -> %s (seq=%d order=%d %dB) %v", it.label, it.pkt.SeqID, it.pkt.OrderID, len(it.pkt.Payload), w.addr)
	}
}

// retransmit resends every buffered reliable packet whose RTO has elapsed (same bytes/seq),
// dropping any that exhausts rudpMaxRetries (the client is gone or will reconnect).
func (w *Writer) retransmit(now time.Time) {
	for seq, p := range w.pending {
		if now.Before(p.deadline) {
			continue
		}
		if p.retries >= rudpMaxRetries {
			delete(w.pending, seq)
			log.Printf("[mm-udp] retransmit gave up on seq=%d (%d tries) %v", seq, p.retries, w.addr)
			continue
		}
		if _, err := w.conn.WriteToUDP(p.wire, w.addr); err != nil {
			log.Printf("[mm-udp] retransmit err: %v", err)
		}
		p.retries++
		p.deadline = now.Add(rudpRTO)
	}
}

// onAck routes a client ACK (of one of our reliable seqs) to run(). Non-blocking: a full
// buffer just delays the ACK, so the packet retransmits once more (harmless — client de-dupes).
func (w *Writer) onAck(seq uint16) {
	select {
	case w.acks <- seq:
	default:
	}
}

// send queues a logical packet; run() assigns its seq/order (if reliable), frames, and sends it,
// logging `label` with the assigned seq/order when non-empty.
func (w *Writer) send(p *packet.Packet, label string) {
	w.ch <- outItem{pkt: p, label: label}
}

// sendRaw queues a pre-encoded packet (e.g. an ack) to emit verbatim.
func (w *Writer) sendRaw(raw []byte) {
	w.ch <- outItem{raw: raw}
}

// frameVar encodes an unreliable VAR packet (send_option=4, plaintext, no seq/order) ONCE.
// VAR framing carries no per-connection state, so the resulting bytes are identical for every
// recipient — this is what lets a match encode a PRI/GRI tick once and fan the SAME slice to all
// players (the key is unused for a plaintext VAR, but Encode takes it). Returns nil on error.
func frameVar(cmd uint16, payload, key []byte) []byte {
	wire, err := (&packet.Packet{SendOption: packet.SendVar, Cmd: cmd, Flags: 0, Payload: payload}).Encode(key)
	if err != nil {
		log.Printf("[mm-udp] frameVar cmd=%d err: %v", cmd, err)
		return nil
	}
	return wire
}

// broadcast enqueues the SAME pre-framed bytes to every recipient Writer (verbatim). Used for the
// VAR replication streams (900/901/1000) whose bytes are identical for all players.
func broadcast(wire []byte, outs []*Writer) {
	if wire == nil {
		return
	}
	for _, out := range outs {
		out.sendRaw(wire)
	}
}

// parseAckSeq extracts the ACKed reliable seq from a client cmd-2 ACK payload
// ([seq u16][ackBits u32], plaintext). Returns false if the payload is too short.
func parseAckSeq(payload []byte) (uint16, bool) {
	if len(payload) < 2 {
		return 0, false
	}
	return binary.LittleEndian.Uint16(payload), true
}
