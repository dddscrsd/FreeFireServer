package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
)

// signForTest mints a token the same way src/utils/jwt.js does, so the Verify
// tests don't depend on the Node signer being present.
func signForTest(claims map[string]any, secret []byte) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	pj, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(pj)
	signingInput := header + "." + payload
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestVerifyRoundTrip(t *testing.T) {
	secret := []byte("dev-match-secret-change-me")
	tok := signForTest(map[string]any{
		"aid": 10000001, "name": "foices", "region": "BR", "mid": 2,
		"show": map[string]any{"avatar": 102000004, "color": 5, "clothes": []int{203000001, 211000000}},
	}, secret)
	c, err := Verify(tok, secret)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if c.AccountID != 10000001 || c.Name != "foices" || c.Region != "BR" || c.MatchID != 2 {
		t.Fatalf("bad claims: %+v", c)
	}
	if c.Show == nil || c.Show.Avatar != 102000004 || len(c.Show.Clothes) != 2 {
		t.Fatalf("bad show: %+v", c.Show)
	}
}

func TestVerifyBadSignature(t *testing.T) {
	tok := signForTest(map[string]any{"aid": 1}, []byte("secretA"))
	if _, err := Verify(tok, []byte("secretB")); err == nil {
		t.Fatalf("want a signature error, got nil")
	}
}

func TestVerifyMalformed(t *testing.T) {
	if _, err := Verify("not.a", []byte("x")); err == nil {
		t.Fatalf("want a malformed-token error, got nil")
	}
}

func TestVerifyExpired(t *testing.T) {
	secret := []byte("s")
	tok := signForTest(map[string]any{"aid": 1, "exp": 1}, secret) // exp in 1970
	if _, err := Verify(tok, secret); err != ErrExpired {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}

// TestVerifyNodeToken checks a token produced by src/utils/jwt.js (passed via
// the MATCH_TEST_TOKEN env var) verifies with the shared dev secret — proving
// the Node signer and Go verifier are wire compatible. Skipped if unset.
func TestVerifyNodeToken(t *testing.T) {
	tok := os.Getenv("MATCH_TEST_TOKEN")
	if tok == "" {
		t.Skip("MATCH_TEST_TOKEN not set")
	}
	c, err := Verify(tok, []byte("dev-match-secret-change-me"))
	if err != nil {
		t.Fatalf("verify node token: %v", err)
	}
	if c.AccountID != 10000001 || c.Name != "foices" {
		t.Fatalf("unexpected node claims: %+v", c)
	}
	t.Logf("node token OK: acc=%d name=%q region=%q mid=%d show=%+v", c.AccountID, c.Name, c.Region, c.MatchID, c.Show)
}
