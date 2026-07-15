// Package token verifies the match prepare_token, an HS256 JWT signed by the
// TCP matchmaker (src/tcp/matchmaker.js) with the shared MATCH_JWT_SECRET. The
// client carries it into the UDP match server in cmd 440 (C2S_RUDP_JoinMatch_Req)
// as the Token field; we verify it here and build cmd 101 PLAYER_JOIN from the
// claims instead of hardcoding the player.
//
// The JWT format MUST stay wire compatible with src/utils/jwt.js: base64url
// (no padding) segments, HMAC-SHA256 over "header.payload".
package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Show carries the player's selected cosmetics (skins / "shows"). Sourced from
// the account's selected_items; mirrors the `show` claim written by matchmaker.js.
type Show struct {
	Avatar  uint32   `json:"avatar"`
	Color   uint32   `json:"color"`
	Head    uint32   `json:"head"`
	Banner  uint32   `json:"banner"`
	Clothes []uint32 `json:"clothes"`
	Slots   []uint32 `json:"slots"`
	Emotes  []uint32 `json:"emotes"`
	// Shows are the account's showcase item ids; the client buckets them by CollectionSubType into
	// BaseProfileInfo.Shows. Shows[7] (weapon) is the DISPLAY weapon on the post-match result screen.
	Shows []uint32 `json:"shows"`
	// BattleFlag is the player's equipped battle-flag config id (BattleFlag.csv Id). Nonzero lets the client
	// plant flag emotes (it gates the cmd 0x114 send). Optional — 0/absent means no equipped flag.
	BattleFlag uint32 `json:"battle_flag"`
}

// Claims is the decoded prepare_token payload.
type Claims struct {
	AccountID uint64 `json:"aid"`
	Name      string `json:"name"`
	Region    string `json:"region"`
	Role      uint32 `json:"role"`
	MatchID   uint64 `json:"mid"`
	Team      uint8  `json:"team"` // custom-room team (group_id 1/2); 0/absent => the match server chooses (balance-fill)
	Show      *Show  `json:"show,omitempty"`
	IssuedAt  int64  `json:"iat"`
	Expiry    int64  `json:"exp"`
}

var (
	ErrMalformed = errors.New("jwt: malformed token")
	ErrSignature = errors.New("jwt: bad signature")
	ErrExpired   = errors.New("jwt: token expired")
)

// Verify checks the HS256 signature (and exp, if present) and returns the claims.
func Verify(tok string, secret []byte) (*Claims, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid parts")
	}

	signingInput := parts[0] + "." + parts[1]

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return nil, errors.New("bad signature")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	var c Claims
	if err := json.Unmarshal(payloadBytes, &c); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	if c.Expiry != 0 && time.Now().Unix() > c.Expiry {
		return nil, ErrExpired
	}

	return &c, nil
}
