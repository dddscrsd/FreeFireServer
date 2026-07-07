package message

// HurtAnim builds the cmd 1010 (message HCHMOBDGGAJ) hit-reaction payload. The server sends it on
// every damage tick so the victim plays its get-hurt flinch on all clients (handler
// KPDMJKOEHEE::FACIFDCFNDJ @0x195e8e4 -> Player::PlayHurt @0x191296c: blood/hit-spark + the flinch
// animation, self-throttled to once/second per victim). Wire = 12 bytes LE (Serialize @0x368eb74):
//
//	[0:4]  u32 victim   — the flinching player's PlayerID (== our entity id: team<<24 | index)
//	[4:8]  u32 attacker — the shooter's PlayerID, used only to pick the weapon's hit-spark; for
//	                      no-attacker damage (zone/environment) pass attacker == victim (default blood)
//	[8:12] i32 gate     — 0 plays the flinch animation; -1 would suppress it (blood only)
func HurtAnim(victim, attacker uint32) []byte {
	w := &Writer{}
	w.U32(victim)
	w.U32(attacker)
	w.U32(0) // reactionGate int32 = 0 -> play the flinch
	return w.B
}
