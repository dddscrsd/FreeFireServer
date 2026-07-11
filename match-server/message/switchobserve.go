package message

// SwitchObserve builds message::EGACIJJMGDK — cmd 149 RUDP_SWITCH_OBSERVE, a SERVER-AUTHORITATIVE
// re-point of a spectating client to a new target. Handler KPDMJKOEHEE::NEKNHKJJBKB @0x194eca8 resolves
// CurrentLocalObserver() and calls its FNCMBMMKLLI::BKBGKJFKBFD(target, apply) @0x2520460 — which only
// performs the switch when `apply` is TRUE (false just clears a flag, no re-point). So send apply=true.
//
// Wire (UnSerialize @0x36d3540, little-endian, 5 bytes):
//
//	bool apply    (AGOFJNKOIBI — must be true to actually switch the observed target)
//	u32  target   (DEIFKHDOLAK — the new observed player/entity id; resolved via NFJPHMKKEBF::KAGPBMINBIJ)
func SwitchObserve(target uint32) []byte {
	w := &Writer{}
	w.Bool(true)  // AGOFJNKOIBI — apply the switch
	w.U32(target) // DEIFKHDOLAK — new observed entity id
	return w.B
}
