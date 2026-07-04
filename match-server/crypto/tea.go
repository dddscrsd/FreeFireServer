package crypto

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
)

// Modified-CBC TEA used for "reliable" UDP payloads (matches fast_crypto.c /
// GCommon::NetworkCryptologyUtil). 16 rounds, delta 0x9E3779B9, 16-byte key.
// Layout of the plaintext before ECB-CBC: [padCount:1][pad zeros][salt:2][data][zeros:7],
// total rounded up to a multiple of 8. The 7 trailing zeros are checked on
// decrypt to detect corruption/wrong key.

const (
	teaDelta  uint32 = 0x9E3779B9
	teaRounds        = 16
	// teaDelta * teaRounds mod 2^32, for decrypt's initial sum.
	teaSumInit uint32 = 0xE3779B90
)

var ErrTeaBadInput = errors.New("tea: bad input length")
var ErrTeaBadPadding = errors.New("tea: zero-padding verification failed")

func keyWords(key []byte) [4]uint32 {
	var k [4]uint32
	for i := 0; i < 4; i++ {
		k[i] = binary.LittleEndian.Uint32(key[i*4:])
	}
	return k
}

func teaEncryptECB(data []byte, k [4]uint32, out []byte) {
	v0 := binary.LittleEndian.Uint32(data[0:])
	v1 := binary.LittleEndian.Uint32(data[4:])
	var sum uint32
	for i := 0; i < teaRounds; i++ {
		sum += teaDelta
		v0 += ((v1 << 4) + k[0]) ^ (v1 + sum) ^ ((v1 >> 5) + k[1])
		v1 += ((v0 << 4) + k[2]) ^ (v0 + sum) ^ ((v0 >> 5) + k[3])
	}
	binary.LittleEndian.PutUint32(out[0:], v0)
	binary.LittleEndian.PutUint32(out[4:], v1)
}

func teaDecryptECB(data []byte, k [4]uint32, out []byte) {
	v0 := binary.LittleEndian.Uint32(data[0:])
	v1 := binary.LittleEndian.Uint32(data[4:])
	sum := teaSumInit
	for i := 0; i < teaRounds; i++ {
		v1 -= ((v0 << 4) + k[2]) ^ (v0 + sum) ^ ((v0 >> 5) + k[3])
		v0 -= ((v1 << 4) + k[0]) ^ (v1 + sum) ^ ((v1 >> 5) + k[1])
		sum -= teaDelta
	}
	binary.LittleEndian.PutUint32(out[0:], v0)
	binary.LittleEndian.PutUint32(out[4:], v1)
}

// TeaEncrypt encrypts plain with the 16-byte key, returning the ciphertext
// (multiple of 8 bytes). Uses a random 2-byte salt.
func TeaEncrypt(plain, key []byte) ([]byte, error) {
	if len(key) != 16 {
		return nil, ErrTeaBadInput
	}
	k := keyWords(key)
	plainLen := len(plain)
	padding := (plainLen + 2 + 7 + 1) % 8
	if padding != 0 {
		padding = 8 - padding
	}
	encLen := ((1 + padding + 2 + plainLen + 7) + 7) / 8 * 8

	buf := make([]byte, encLen)
	pos := 0
	buf[pos] = byte(padding)
	pos++
	pos += padding // pad zeros (already zero)
	var salt [2]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return nil, err
	}
	buf[pos] = salt[0]
	buf[pos+1] = salt[1]
	pos += 2
	copy(buf[pos:], plain)
	// trailing 7 zero bytes already zero

	out := make([]byte, encLen)
	var ivPlain, ivCrypt, blockXored, enc [8]byte
	for blk := 0; blk < encLen; blk += 8 {
		for i := 0; i < 8; i++ {
			blockXored[i] = buf[blk+i] ^ ivCrypt[i]
		}
		teaEncryptECB(blockXored[:], k, enc[:])
		for i := 0; i < 8; i++ {
			enc[i] ^= ivPlain[i]
			out[blk+i] = enc[i]
		}
		copy(ivPlain[:], blockXored[:])
		copy(ivCrypt[:], out[blk:blk+8])
	}
	return out, nil
}

// TeaDecrypt reverses TeaEncrypt. Returns the plaintext, or an error on bad
// length / failed zero-padding check (wrong key or corruption).
func TeaDecrypt(cipher, key []byte) ([]byte, error) {
	if len(key) != 16 {
		return nil, ErrTeaBadInput
	}
	if len(cipher) < 16 || len(cipher)&7 != 0 {
		return nil, ErrTeaBadInput
	}
	k := keyWords(key)

	var decDest [8]byte
	teaDecryptECB(cipher[0:8], k, decDest[:])

	padding := int(decDest[0] & 7)
	plainLen := len(cipher) - 1 - padding - 2 - 7
	if plainLen < 0 {
		return nil, ErrTeaBadInput
	}
	out := make([]byte, plainLen)

	var ivPre, ivCur [8]byte
	copy(ivCur[:], cipher[0:8])
	cipherPos := 8
	pos := 1 + padding

	advance := func() error {
		if cipherPos+8 > len(cipher) {
			return ErrTeaBadInput
		}
		copy(ivPre[:], ivCur[:])
		copy(ivCur[:], cipher[cipherPos:cipherPos+8])
		for i := 0; i < 8; i++ {
			decDest[i] ^= cipher[cipherPos+i]
		}
		teaDecryptECB(decDest[:], k, decDest[:])
		cipherPos += 8
		pos = 0
		return nil
	}

	// skip the 2 salt bytes
	for saltCount := 1; saltCount <= 2; {
		if pos == 8 {
			if err := advance(); err != nil {
				return nil, err
			}
		} else {
			pos++
			saltCount++
		}
	}
	// extract data
	for dataCount := 0; dataCount < plainLen; {
		if pos <= 7 {
			out[dataCount] = decDest[pos] ^ ivPre[pos]
			pos++
			dataCount++
		} else if err := advance(); err != nil {
			return nil, err
		}
	}
	// verify the 7 trailing zero bytes
	for zeroCount := 1; zeroCount <= 7; {
		if pos <= 7 {
			if decDest[pos]^ivPre[pos] != 0 {
				return nil, ErrTeaBadPadding
			}
			pos++
			zeroCount++
		} else if err := advance(); err != nil {
			return nil, err
		}
	}
	return out, nil
}
