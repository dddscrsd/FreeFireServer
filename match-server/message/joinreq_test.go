package message

import (
	"encoding/binary"
	"encoding/hex"
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

// foices440Hex is a real cmd 440 (JOIN_MATCH_POST) payload captured when "foices" joined
// with a large prepare_token split across cmd 439/440. The token's tail — the remaining
// 17 signature chars — is the trailing length-prefixed field ("-TTIfydNeKqkyRKlA").
const foices440Hex = "81969800000000000700000000000000010f07000000000000000100000000000000000101020300072200000068747470733a2f2f62726e6574776f726b2e6767626c7565736861726b2e636f6d2f6400000001000000000406000000312e37302e310a0000003230313931313439373100000000819698000000000006000000666f696365730000000000000000000000000000110000002d5454496679644e654b716b79524b6c41"

func TestLastJWTFieldSplitTail(t *testing.T) {
	b, err := hex.DecodeString(foices440Hex)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := LastJWTField(b), "-TTIfydNeKqkyRKlA"; got != want {
		t.Fatalf("LastJWTField = %q, want %q (must skip metadata: host, version, nick)", got, want)
	}
}

func TestIsCompleteJWT(t *testing.T) {
	partialSig := jwtHeaderMarker + ".eyJhaWQiOjF9.qBHR-qMQPB_puvHlSxfE2Ivtnw" // 26-char sig (the 439 chunk)
	if IsCompleteJWT(partialSig) {
		t.Fatal("a truncated 26-char signature must read as incomplete")
	}
	if !IsCompleteJWT(partialSig + "-TTIfydNeKqkyRKlA") { // +17 = 43-char HS256 sig
		t.Fatal("a full 43-char signature must read as complete")
	}
	if IsCompleteJWT("header.payload") {
		t.Fatal("two segments (split mid-payload) must read as incomplete")
	}
}

// TestReassembleSplitToken mirrors handleJoinMatchPost's reassembly: the 439 chunk
// (missing its signature tail) plus the 440 tail must form a complete JWT.
func TestReassembleSplitToken(t *testing.T) {
	b, _ := hex.DecodeString(foices440Hex)
	chunk439 := jwtHeaderMarker + ".eyJhaWQiOjF9.qBHR-qMQPB_puvHlSxfE2Ivtnw" // 26-char partial sig
	tok := ExtractJWT([]byte(chunk439))
	if !IsCompleteJWT(tok) {
		tok += LastJWTField(b)
	}
	if !IsCompleteJWT(tok) {
		t.Fatalf("reassembled token still incomplete: %q", tok)
	}
	if want := chunk439 + "-TTIfydNeKqkyRKlA"; tok != want {
		t.Fatalf("reassembled = %q, want %q", tok, want)
	}
}
