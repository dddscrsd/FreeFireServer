package message

import (
	"encoding/binary"
	"testing"
)

// buildJoinReq encodes a C2S_RUDP_JoinMatch_Req the way the client does, so we
// can assert ParseJoinMatchReq recovers the header + Token.
func buildJoinReq(userID uint64, roomID uint32, roomType, gameMode uint8, serviceRoom uint64, tok string) []byte {
	b := make([]byte, 0, 22+4+len(tok)+8)
	b = binary.LittleEndian.AppendUint64(b, userID)
	b = binary.LittleEndian.AppendUint32(b, roomID)
	b = append(b, roomType, gameMode)
	b = binary.LittleEndian.AppendUint64(b, serviceRoom)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(tok)))
	b = append(b, tok...)
	// trailing optional fields (map_id, is_reconnect) — must be ignored
	b = binary.LittleEndian.AppendUint32(b, 1)
	b = append(b, 0)
	return b
}

func TestParseJoinMatchReq(t *testing.T) {
	tok := "eyJhbGci.payload.sig"
	b := buildJoinReq(10000001, 42, 1, 15, 777, tok)
	r, err := ParseJoinMatchReq(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.UserID != 10000001 || r.RoomID != 42 || r.RoomType != 1 || r.GameMode != 15 || r.ServiceRoomID != 777 {
		t.Fatalf("bad header: %+v", r)
	}
	if r.Token != tok {
		t.Fatalf("bad token: %q", r.Token)
	}
}

func TestParseJoinMatchReqNoToken(t *testing.T) {
	// Header only (no Token field): should parse with an empty Token, no error.
	b := buildJoinReq(5, 6, 0, 0, 0, "")
	b = b[:joinReqHeaderLen] // drop the len-prefix + trailing fields
	r, err := ParseJoinMatchReq(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.Token != "" || r.UserID != 5 {
		t.Fatalf("expected empty token, got %+v", r)
	}
}

func TestParseJoinMatchReqShort(t *testing.T) {
	if _, err := ParseJoinMatchReq([]byte{1, 2, 3}); err != ErrJoinReqShort {
		t.Fatalf("want ErrJoinReqShort, got %v", err)
	}
}

func TestExtractJWT(t *testing.T) {
	tok := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhaWQiOjF9.abc-DEF_123"
	// Embed the token in the middle of arbitrary binary noise (with other bytes
	// on both sides, including a NUL that ends the run).
	payload := append([]byte{0x6c, 0x00, 0xff, 0x01}, tok...)
	payload = append(payload, 0x00, 0xde, 0xad)
	got := ExtractJWT(payload)
	if got != tok {
		t.Fatalf("ExtractJWT = %q, want %q", got, tok)
	}
}

func TestExtractJWTAbsent(t *testing.T) {
	if got := ExtractJWT([]byte("mm-10000001-2\x00\x01")); got != "" {
		t.Fatalf("want empty (no JWT marker), got %q", got)
	}
}

func TestParseJoinMatchReqBogusLen(t *testing.T) {
	// A wildly large Token length must not panic; Token stays empty.
	b := make([]byte, joinReqHeaderLen)
	b = binary.LittleEndian.AppendUint32(b, 0xFFFFFFFF)
	r, err := ParseJoinMatchReq(b)
	if err != nil || r.Token != "" {
		t.Fatalf("want empty token no error, got token=%q err=%v", r.Token, err)
	}
}
