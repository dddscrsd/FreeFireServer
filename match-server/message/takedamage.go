package message

// TakeDamage builds message::CHDLJFJCPFN (cmd 106 RUDP_TAKE_DAMAGE) — the S2C hit notification we
// forward to the VICTIM so its client renders the hit-direction indicator. Field order + types are
// the client's own UnSerialize (message::CHDLJFJCPFN::UnSerialize @0x37e7c50), the SAME class the
// client sends C2S (our combat.go parses it: victim@0, netDmg@4, damage|bodyPart@6, attacker@10).
//
// The previous stub only wrote a short misaligned prefix, so the client read the trailing DAMAGE-TYPE
// bytes past the buffer as garbage — and UIHudHurtHintController::OnLocalPlayerBeHit picks the
// wall-PENETRATION hurt hint whenever the damage type's bit 0 is set, so a random tail byte made it
// mostly show the penetrate arrow instead of the normal one. Writing the full message with the
// damage-type fields (ACAKHEABPEJ / MJIHLDJNHLF / MBGCAHPACOH) = 0 pins the NORMAL hint. The direction
// arrow is derived from the ATTACKER's live position (looked up by LIIGLCNGOHG), not the hit vectors,
// so those stay zero.
func TakeDamage(victim, attacker, weapon uint32, damage uint16, bodyPart int8) []byte {
	bp := uint8(bodyPart)
	w := &Writer{}
	w.U32(victim)                          // ALFINFGBOBE — victim id
	w.U16(damage)                          // ECDBFHHNPMI — net (applied) damage
	w.U32(uint32(damage) | uint32(bp)<<24) // BJBPPEBIPFA — raw damage (low 24) | bodyPart (high 8)
	w.U32(attacker)                        // LIIGLCNGOHG — attacker id (drives the hit-direction arrow)
	w.I32(int32(weapon))                   // PIAMIOFEBKF — weapon
	w.U32(0)                               // HCMIEJEBKAL
	w.U8(bp)                               // ODCJPCEJHPK — hit body part
	w.U32(0)                               // CEDJCPLOLNE — client tick (unused for the indicator)
	w.vec3i(0, 0, 0)                       // CNEICNJFGLM — hit position (DEACEIFBHJK)
	w.vec3i(0, 0, 0)                       // PGDEDHFOMCN — hit direction (DEACEIFBHJK)
	w.I16(0)                               // AALHLOAJLEE — List<float> segment damages (count 0)
	w.U32(0)                               // HOBOHHJNDNH
	w.F32(0)                               // AILHIPMKJKJ
	w.U64(0)                               // LHGGPCFJNOO
	w.I8(0)                                // ACAKHEABPEJ — damage-type byte: 0 = NORMAL (bit 0 = wall-penetrate)
	w.Bool(false)                          // MJIHLDJNHLF — damage-type sub-flag
	w.Bool(false)                          // MBGCAHPACOH — damage-type sub-flag
	w.I16(0)                               // FIKOAMIDEHL — List<byte> anti-cheat blob (count 0)
	w.F32(0)                               // IOGIIEFAALP
	w.Bool(false)                          // HDEJLJKNLCI
	return w.B
}
