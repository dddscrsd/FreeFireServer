package crypto

// CRC7 used by the FF UDP match protocol (GCommon::UDPChecksum::CRC7 /
// fast_crypto.c). 7-bit CRC, polynomial 0x09, computed over the packet bytes
// from offset 2 (send_option) to the end of the payload.

var crc7Table [128][256]byte

func init() {
	for i := 0; i < 128; i++ {
		for j := 0; j < 256; j++ {
			crc := byte(i)
			data := byte(j)
			for bit := 0; bit < 8; bit++ {
				var poly byte
				if (((crc >> 6) ^ (data >> 7)) & 1) != 0 {
					poly = 0x09
				}
				crc = (crc << 1) ^ poly
				crc &= 0x7F
				data <<= 1
			}
			crc7Table[i][j] = crc
		}
	}
}

// CRC7 folds buf into the running 7-bit CRC starting from seed (masked to 7 bits).
func CRC7(seed byte, buf []byte) byte {
	c := seed & 0x7F
	for _, b := range buf {
		c = crc7Table[c][b]
	}
	return c & 0x7F
}
