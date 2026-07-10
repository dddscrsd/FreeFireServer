package main

import (
	"net"
	"testing"
	"time"

	"libmadoka/match-server/packet"
)

// TestWriterRetransmit exercises the RUDP reliability: a reliable (so=2) send is buffered
// and resent while un-ACKed, and an ACK stops the retransmits.
func TestWriterRetransmit(t *testing.T) {
	rconn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer rconn.Close()
	sconn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer sconn.Close()

	key := make([]byte, 16)
	w := newWriter(sconn, rconn.LocalAddr().(*net.UDPAddr), key)

	readOne := func(d time.Duration) (*packet.Packet, bool) {
		_ = rconn.SetReadDeadline(time.Now().Add(d))
		buf := make([]byte, 2048)
		n, _, err := rconn.ReadFromUDP(buf)
		if err != nil {
			return nil, false
		}
		p, err := packet.Decode(buf[:n], key)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		return p, true
	}

	// A reliable death packet: sent once immediately, buffered for retransmit.
	w.send(&packet.Packet{SendOption: packet.SendReliable, Cmd: packet.CmdDead, Payload: []byte("die")}, "")
	first, ok := readOne(time.Second)
	if !ok {
		t.Fatal("no first send")
	}
	if first.Cmd != packet.CmdDead {
		t.Fatalf("cmd=%d want %d", first.Cmd, packet.CmdDead)
	}
	seq := first.SeqID

	// Un-ACKed -> it must be retransmitted (same seq) after ~RTO.
	rx, ok := readOne(rudpRTO + 300*time.Millisecond)
	if !ok {
		t.Fatal("expected a retransmit of the un-ACKed reliable packet")
	}
	if rx.SeqID != seq {
		t.Fatalf("retransmit seq=%d want %d", rx.SeqID, seq)
	}

	// ACK it -> no further retransmits.
	w.onAck(seq)
	if p, ok := readOne(rudpRTO + 300*time.Millisecond); ok {
		t.Fatalf("got a retransmit (seq=%d) after ACK", p.SeqID)
	}
}

// TestWriterUnreliableNotBuffered confirms a VAR (unreliable) send is never retransmitted.
func TestWriterUnreliableNotBuffered(t *testing.T) {
	rconn, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	defer rconn.Close()
	sconn, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	defer sconn.Close()

	key := make([]byte, 16)
	w := newWriter(sconn, rconn.LocalAddr().(*net.UDPAddr), key)
	w.send(&packet.Packet{SendOption: packet.SendVar, Cmd: packet.CmdPRISync, Payload: []byte("pri")}, "")

	_ = rconn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := rconn.ReadFromUDP(make([]byte, 512)); err != nil {
		t.Fatal("no first VAR send")
	}
	// No retransmit for unreliable.
	_ = rconn.SetReadDeadline(time.Now().Add(rudpRTO + 300*time.Millisecond))
	if _, _, err := rconn.ReadFromUDP(make([]byte, 512)); err == nil {
		t.Fatal("unreliable VAR must not be retransmitted")
	}
}
