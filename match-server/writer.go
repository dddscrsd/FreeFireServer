package main

import (
	"log"
	"net"

	"libmadoka/match-server/packet"
)

// Writer is one connection's outbound path. A single goroutine (run) owns the seq/order
// counters and every WriteToUDP for this remote, so senders never take a lock — they just
// enqueue. Framing in one place is also the fan-out primitive the multiplayer broadcast is
// built on: a match will hand the same pre-framed VAR bytes to every player's Writer. For now
// it serves exactly one session per connection, byte-for-byte identical to the old inline sends.
type Writer struct {
	conn *net.UDPConn
	addr *net.UDPAddr
	key  []byte
	ch   chan outItem

	// Owned SOLELY by run(): the reliable sequence counter (shared by hello + reliable data)
	// and the send_option=2 data order counter. No mutex — run() is the only mutator.
	seq   uint16
	order uint16
}

// outItem is one queued send: either a pre-encoded packet emitted verbatim (raw — e.g. an ack)
// or a logical packet the Writer frames with this connection's next seq/order. label, when set,
// is logged with the assigned seq/order after the send (replaces the old sendDataLog logging).
type outItem struct {
	pkt   *packet.Packet
	raw   []byte
	label string
}

// newWriter starts a Writer and its run() goroutine for the given remote.
func newWriter(conn *net.UDPConn, addr *net.UDPAddr, key []byte) *Writer {
	w := &Writer{conn: conn, addr: addr, key: key, ch: make(chan outItem, 256)}
	go w.run()
	return w
}

// run is the sole owner of seq/order and the socket for this remote: it frames + sends each
// queued item in enqueue (FIFO) order, so the reliable seq/order stream stays monotonic without
// a lock. Enqueue order equals call order, so this preserves the previous per-call seq behavior.
func (w *Writer) run() {
	for it := range w.ch {
		if it.pkt == nil { // pre-encoded (ack): emit verbatim
			if it.raw != nil {
				if _, err := w.conn.WriteToUDP(it.raw, w.addr); err != nil {
					log.Printf("[mm-udp] write err: %v", err)
				}
			}
			continue
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
			continue
		}
		if _, err := w.conn.WriteToUDP(wire, w.addr); err != nil {
			log.Printf("[mm-udp] write err: %v", err)
		}
		if it.label != "" {
			log.Printf("[mm-udp] -> %s (seq=%d order=%d %dB) %v", it.label, it.pkt.SeqID, it.pkt.OrderID, len(it.pkt.Payload), w.addr)
		}
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
