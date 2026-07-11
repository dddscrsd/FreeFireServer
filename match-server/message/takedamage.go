package message

// TakeDamage builds message::OLKFGHFALCN — the S2C response to cmd 106 RUDP_TAKE_DAMAGE that we
// forward to the VICTIM so its client renders the hit-direction indicator (this is a DIFFERENT class
// from the C2S hit report CHDLJFJCPFN; the handler is KPDMJKOEHEE::DPLEHMLOILA @0x194680c, wire =
// UnSerialize @0x34710f0).
//
// Field layout (all little-endian):
//
//	u32 LIIGLCNGOHG  attacker id      (event data[0] -> hit-direction arrow)
//	u32 GCHBDBPGLII  victim id        (event data[3])
//	u16 BJBPPEBIPFA  damage           (event data[1])
//	i32 BJDEIHKKJIC  weapon           (event data[2]; -10 == burning/zone)
//	i32 NKKFOEICEEC  DoT flag         (event data[5]; >=1 shows the DoT hurt effect)
//	i8  ACAKHEABPEJ  DAMAGE TYPE      (event data[4], an ELMGJKHIIAA bitmask)
//
// ACAKHEABPEJ is the damage TYPE, NOT the hit body part (OLKFGHFALCN carries no body-part field). The
// hurt-hint controller (UIHudHurtHintController::OnLocalPlayerBeHit) picks the wall-PENETRATION arrow
// when bit 0 is set (HBCJKAHHBGE(type, 1) == (type & 1)); bit 1 plays the Hayato armor-protection hit
// sound. So it MUST be 0 for an ordinary hit — feeding it the body part (HEAD=1/LIMB=3/PROT=5 are odd)
// was what made the client "mostly" show the penetrate indicator.
func TakeDamage(victim, attacker, weapon uint32, damage uint16, bodyPart int8) []byte {
	_ = bodyPart // not part of this message: OLKFGHFALCN has no body-part field
	w := &Writer{}
	w.U32(attacker)      // LIIGLCNGOHG
	w.U32(victim)        // GCHBDBPGLII
	w.U16(damage)        // BJBPPEBIPFA
	w.I32(int32(weapon)) // BJDEIHKKJIC
	w.I32(0)             // NKKFOEICEEC — no damage-over-time effect
	w.I8(0)              // ACAKHEABPEJ — damage type 0 = normal hit (bit0=wall-penetrate, bit1=Hayato protection)
	return w.B
}
