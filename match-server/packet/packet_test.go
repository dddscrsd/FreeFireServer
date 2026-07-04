package packet

import (
	"bytes"
	"encoding/hex"
	"testing"
)

var key, _ = hex.DecodeString("00112233445566778899aabbccddeeff")

func TestEncodeDecodeUnreliable(t *testing.T) {
	orig := &Packet{SendOption: SendUnreliable, Cmd: 42, Payload: []byte("abc")}
	wire, err := orig.Encode(key)
	if err != nil {
		t.Fatal(err)
	}
	if wire[0] != MsgKey {
		t.Fatalf("msg_key = %#x, want 0x6C", wire[0])
	}
	got, err := Decode(wire, key)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cmd != 42 || got.SendOption != SendUnreliable || !bytes.Equal(got.Payload, []byte("abc")) {
		t.Fatalf("mismatch: %+v", got)
	}
}

func TestEncodeDecodeReliableEncrypted(t *testing.T) {
	orig := &Packet{SendOption: SendReliable, Cmd: 100, SeqID: 7, OrderID: 3, Flags: FlagEncrypted, Payload: []byte("join-match-body")}
	wire, err := orig.Encode(key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire, key)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cmd != 100 || got.SeqID != 7 || got.OrderID != 3 || got.Flags != FlagEncrypted {
		t.Fatalf("header mismatch: %+v", got)
	}
	if !bytes.Equal(got.Payload, []byte("join-match-body")) {
		t.Fatalf("payload mismatch: %q", got.Payload)
	}
}

func TestCRCRejection(t *testing.T) {
	wire, _ := (&Packet{SendOption: SendReliable, Cmd: 5, SeqID: 1, OrderID: 1, Payload: []byte("x")}).Encode(key)
	wire[len(wire)-1] ^= 0xFF // corrupt payload
	if _, err := Decode(wire, key); err != ErrCRC {
		t.Fatalf("want ErrCRC, got %v", err)
	}
}

func TestDecodeClientHello(t *testing.T) {
	// what the client sends on connect: cmd=1, send_option=1 (reliable), empty payload.
	hello := &Packet{SendOption: SendHello, Cmd: CmdHello, SeqID: 0, OrderID: 0, Payload: nil}
	wire, _ := hello.Encode(key)
	got, err := Decode(wire, key)
	if err != nil {
		t.Fatal(err)
	}
	if !IsReliable(got.Cmd, got.SendOption) {
		t.Fatal("hello should be reliable")
	}
	if got.Cmd != CmdHello || got.SendOption != SendHello || len(got.Payload) != 0 {
		t.Fatalf("hello mismatch: %+v", got)
	}
}
