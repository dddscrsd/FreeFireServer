package message

import (
	"encoding/binary"
	"math"
	"math/rand"
	"time"
)

// Fixed-width little-endian writer, matching GCommon::FastBinaryWriter when
// UDPParameters.ENABLE_FAST_PROTO == false (the mode our S2C_Hello_Res selects,
// so the client reads TryReadFixU32/16/etc. rather than varints). Strings are
// int32 length prefix + UTF-8 bytes (GCommon::UDPClientMessageBase::ReadString).
type Writer struct{ B []byte }

func (w *Writer) U8(v byte) { w.B = append(w.B, v) }
func (w *Writer) Bool(v bool) {
	if v {
		w.U8(1)
	} else {
		w.U8(0)
	}
}
func (w *Writer) U16(v uint16)  { w.B = binary.LittleEndian.AppendUint16(w.B, v) }
func (w *Writer) I16(v int16)   { w.U16(uint16(v)) }
func (w *Writer) U32(v uint32)  { w.B = binary.LittleEndian.AppendUint32(w.B, v) }
func (w *Writer) I32(v int32)   { w.U32(uint32(v)) }
func (w *Writer) U64(v uint64)  { w.B = binary.LittleEndian.AppendUint64(w.B, v) }
func (w *Writer) F32(v float32) { w.U32(math.Float32bits(v)) }
func (w *Writer) Str(s string)  { w.I32(int32(len(s))); w.B = append(w.B, s...) }

// vec3i writes a message::DEACEIFBHJK (3x int32).
func (w *Writer) vec3i(x, y, z int32) { w.I32(x); w.I32(y); w.I32(z) }

// U32List writes a plain List<u32>: an int16 count followed by the elements.
// (The "read-ahead" cosmetic lists MCPGFPHMOGM/GLHCLEMMLDN are NOT plain lists —
// the client always reads one trailing u32 after the count — so write those by
// hand as i16(count)+elements+u32(0).)
func (w *Writer) U32List(v []uint32) {
	w.I16(int16(len(v)))
	for _, x := range v {
		w.U32(x)
	}
}

// JoinMatchRes builds message::LGIGCGIDOKP (cmd 100), the join result the client
// deserializes in GLPOGGHMJAB. Field order matches LGIGCGIDOKP::UnSerialize.
// result=0 is the success code (message::PHIFIIFMJKP); the rest are left at
// their zero/empty defaults for now (lists empty, nested messages zeroed).
func JoinMatchRes(result uint32) []byte {
	w := &Writer{}
	w.U32(result)                    // MBBDNIBDPME  (join result enum)
	w.U64(1)                         // DGCJANAMDBJ
	w.U64(0)                         // LKNFCCLNBCO
	w.U64(0)                         // KLGIHHCLFII
	w.I32(0)                         // EGOMKONIBMP
	w.U8(3)                          // KHEFHJNNLIL
	w.U32(uint32(time.Now().Unix())) // HINHHMCOFLD

	choices := []int{
		5000, 10000, 15000, 20000, 25000, 30000, 35000,
		40000, 45000, 50000, 55000, 60000, 65000,
	}

	value := choices[rand.Intn(len(choices))]
	w.U16(uint16(value)) // FMMDBCOBGDM
	w.U32(0)             // CKAAOCLHHMM
	w.U32(0)             // DPOPAMPEKGA
	// MDGGALNBIKP : PGKJDKIJGJD { i16, DEACEIFBHJK, DEACEIFBHJK, f32, f32 }
	w.I16(0)
	w.vec3i(0, 0, 0)
	w.vec3i(0, 0, 0)
	w.F32(0)
	w.F32(0)
	w.I16(0)      // GJGPMHJJOCE : List<i32>  (count=0)
	w.I16(0)      // KBCPCCKGGBE : List<u32>  (count=0)
	w.I16(0)      // KELMCBLFFDC : List<msg>  (count=0)
	w.U32(0)      // GPKHLEAADHL
	w.U32(0)      // OJIHIKIAFAI
	w.Bool(true)  // FEAMHOEGDAG
	w.Bool(false) // OEFEABKDIFM
	w.Bool(false) // NBFOFDCIFOG
	w.Bool(false) // EHFODNFPJMM
	w.Bool(false) // PLGGBGOCBCK
	w.U32(0)      // DFEIAPMNJCK
	w.Bool(false) // ECAGPHCIFFB
	w.Bool(false) // NNMLDEJEJIM
	w.Bool(false) // NALEHBGHPOP
	// PGHKLEAHENK : EKHEMGJLHDC { string, string, bool, List<byte> }
	w.Str("")
	w.Str("")
	w.Bool(false)
	w.I16(0)      // LCECHEFDEDI byte-list count
	w.Bool(false) // GHEACGCMOHK
	return w.B
}
