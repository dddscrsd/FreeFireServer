package message

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// TestPlayerJoinIdentity checks that PlayerJoin encodes the identity at the head
// of OGJKHJAFNHB (u64 account id | u64 | u32 entity | i32 name-len + name), so
// wiring cosmetics later can't silently corrupt the fields the client keys on.
func TestPlayerJoinIdentity(t *testing.T) {
	pi := PlayerInfo{AccountID: 10000001, EntityID: 1, Name: "foices"}
	b := PlayerJoin(pi)

	if got := binary.LittleEndian.Uint64(b[0:8]); got != pi.AccountID {
		t.Fatalf("account id = %d, want %d", got, pi.AccountID)
	}
	if got := binary.LittleEndian.Uint32(b[16:20]); got != pi.EntityID {
		t.Fatalf("entity id = %d, want %d", got, pi.EntityID)
	}
	nameLen := int(binary.LittleEndian.Uint32(b[20:24]))
	if nameLen != len(pi.Name) || string(b[24:24+nameLen]) != pi.Name {
		t.Fatalf("name = %q (len %d), want %q", b[24:24+nameLen], nameLen, pi.Name)
	}
}

// TestPlayerJoinStableWithoutCosmetics documents the known-good baseline length
// (cosmetics unset ⇒ all-zero PKPAMKEDCDC). If this changes, the join wire
// format changed — make sure the client still accepts it.
func TestPlayerJoinStableWithoutCosmetics(t *testing.T) {
	b := PlayerJoin(PlayerInfo{AccountID: 10000001, EntityID: 1, Name: "foices"})
	const wantLen = 234 // verified accepted live (see UDP_MATCH_PROTOCOL.md)
	if len(b) != wantLen {
		t.Fatalf("PlayerJoin len = %d, want %d (baseline changed — reverify with the client)", len(b), wantLen)
	}
}

func TestU32List(t *testing.T) {
	var w Writer
	w.U32List([]uint32{203000001, 211000000})
	want := []byte{0x02, 0x00} // int16 count = 2
	want = binary.LittleEndian.AppendUint32(want, 203000001)
	want = binary.LittleEndian.AppendUint32(want, 211000000)
	if !bytes.Equal(w.B, want) {
		t.Fatalf("U32List = % x, want % x", w.B, want)
	}
}
