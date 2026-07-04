package message

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// JoinMatchReq is the subset of C2S_RUDP_JoinMatch_Req (cmd 440) we consume.
// Layout reverse-engineered in original_to_read/udp.py:deserialize_join_match_req:
//
//	u64 UserID | u32 RoomID | u8 RoomType | u8 GameMode | u64 ServiceRoomID |
//	string Token (u32 len + UTF-8) | ...optional fields (map_id, is_reconnect, ...)
//
// Token is our prepare_token (a JWT) — the client echoes back the value we put
// in MatchmakingSussNtf.prepare_token. We only need the header fields + Token;
// the trailing optional fields are ignored.
type JoinMatchReq struct {
	UserID        uint64
	RoomID        uint32
	RoomType      uint8
	GameMode      uint8
	ServiceRoomID uint64
	Token         string
}

// ErrJoinReqShort means the packet is too small to hold the fixed header.
var ErrJoinReqShort = errors.New("joinreq: too short")

const joinReqHeaderLen = 8 + 4 + 1 + 1 + 8 // UserID..ServiceRoomID

// maxTokenLen bounds the Token string so a corrupt length prefix can't make us
// slice out of range (mirrors the reference's 1000-byte sanity cap, relaxed to
// comfortably fit a JWT).
const maxTokenLen = 4096

// ParseJoinMatchReq decodes the fixed header and the Token string. A missing or
// malformed Token is not an error: the header fields are returned with an empty
// Token so the caller can fall back to defaults.
func ParseJoinMatchReq(b []byte) (*JoinMatchReq, error) {
	if len(b) < joinReqHeaderLen {
		return nil, ErrJoinReqShort
	}
	off := 0
	r := &JoinMatchReq{}
	r.UserID = binary.LittleEndian.Uint64(b[off:])
	off += 8
	r.RoomID = binary.LittleEndian.Uint32(b[off:])
	off += 4
	r.RoomType = b[off]
	off++
	r.GameMode = b[off]
	off++
	r.ServiceRoomID = binary.LittleEndian.Uint64(b[off:])
	off += 8

	if off+4 > len(b) {
		return r, nil // no Token field present
	}
	tokLen := int(binary.LittleEndian.Uint32(b[off:]))
	off += 4
	if tokLen <= 0 || tokLen > maxTokenLen || off+tokLen > len(b) {
		return r, nil // Token absent / length implausible — leave it empty
	}
	r.Token = string(b[off : off+tokLen])
	return r, nil
}

// jwtHeaderMarker is the fixed base64url encoding of the JWT header our signer
// always emits: {"alg":"HS256","typ":"JWT"}. Because it's constant, we can find
// our prepare_token anywhere in the cmd 440 payload without depending on the
// exact (unverified for 1.70) surrounding struct layout.
const jwtHeaderMarker = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"

func isJWTChar(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_'
}

// ExtractJWT scans b for an embedded HS256 JWT (our prepare_token): it locates
// the fixed header marker and returns the maximal following run of JWT
// characters ([A-Za-z0-9._-]). This is a layout-independent fallback for when
// the cmd 440 struct offsets don't line up. Returns "" if no token is found.
func ExtractJWT(b []byte) string {
	i := bytes.Index(b, []byte(jwtHeaderMarker))
	if i < 0 {
		return ""
	}
	j := i
	for j < len(b) && isJWTChar(b[j]) {
		j++
	}
	return string(b[i:j])
}
