package crypto

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// The stub match secret (hex) the matchmaker hands out -> 16-byte TEA key.
var testKey, _ = hex.DecodeString("00112233445566778899aabbccddeeff")

func TestTeaRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 5, 7, 8, 9, 16, 31, 100, 255} {
		plain := make([]byte, n)
		for i := range plain {
			plain[i] = byte(i*7 + 3)
		}
		ct, err := TeaEncrypt(plain, testKey)
		if err != nil {
			t.Fatalf("encrypt n=%d: %v", n, err)
		}
		if len(ct)%8 != 0 || len(ct) < 8 {
			t.Fatalf("n=%d ciphertext len %d not a positive multiple of 8", n, len(ct))
		}
		pt, err := TeaDecrypt(ct, testKey)
		if err != nil {
			t.Fatalf("decrypt n=%d: %v", n, err)
		}
		if !bytes.Equal(pt, plain) {
			t.Fatalf("n=%d round-trip mismatch: got %x want %x", n, pt, plain)
		}
	}
}

func TestTeaWrongKeyFailsPadding(t *testing.T) {
	ct, _ := TeaEncrypt([]byte("hello match server"), testKey)
	bad := make([]byte, 16)
	copy(bad, testKey)
	bad[0] ^= 0xFF
	if _, err := TeaDecrypt(ct, bad); err == nil {
		t.Fatal("expected padding failure with wrong key")
	}
}

func TestCRC7Deterministic(t *testing.T) {
	// send_option=0, cmd=1 (LE), flags=0, len=0 -> unreliable HELLO-style body
	body := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x00}
	a := CRC7(0, body)
	b := CRC7(0, body)
	if a != b {
		t.Fatalf("CRC7 not deterministic: %d vs %d", a, b)
	}
	if a > 0x7F {
		t.Fatalf("CRC7 not 7-bit: %d", a)
	}
	// folding one byte at a time must equal folding the whole buffer.
	c := byte(0)
	for _, x := range body {
		c = CRC7(c, []byte{x})
	}
	if c != a {
		t.Fatalf("CRC7 incremental %d != whole %d", c, a)
	}
	t.Logf("CRC7(send_option=0,cmd=1,flags=0,len=0) = %d", a)
}
