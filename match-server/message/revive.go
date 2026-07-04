package message

// NotifyRevive builds the cmd 388 (RUDP_NOTIFYREVIVE, message MJCMIMNHILD) payload: it
// revives an EXISTING dead player (by id) alive at a position, KEEPING its team, faction
// and loadout — because the client reuses the same Player object (handler DLEEFMJPCHA
// @0x1a49080 looks it up, sets IsDead=0 via StopPendingRevive, and repositions it via
// ReviveInitMotionState). This is the correct CS round-respawn — unlike a cmd 101
// re-join, which re-runs LONDNBHBPDO (re-add + BindPRI) and disrupts the bot's team
// colour + wipes the loadout to fists. HP is NOT set here; the (still-bound) PRI stream
// supplies it. For the LOCAL player, the reposition is a client-side self-teleport that
// sticks. Wire order verified from MJCMIMNHILD::UnSerialize @0x348e970, sub-message
// KLAAKHJCCEN @0x3732f94, and DEACEIFBHJK(Vec3) @0x36c6e9c. See [[bot-networkaipawn]].
//
//	u32 reviverId | u32 revivedId | i32 x_mm | i32 y_mm | i32 z_mm | u8 yaw | u32 param
//
// reviverId is cosmetic ("who revived"); for a self-respawn we set it = revivedId. yaw is
// 0..255 mapped to 0..360°. param (motion/init state) is 0.
func NotifyRevive(revivedID uint32, pos Vec3, yaw byte) []byte {
	w := &Writer{}
	w.U32(revivedID)                                  // AOHKECDBLGA  reviver id (cosmetic; self-revive)
	w.U32(revivedID)                                  // OBPAEHCENFE  revived/target player id
	w.vec3i(r1000(pos.X), r1000(pos.Y), r1000(pos.Z)) // CCIKDFGDBAM  revive position (DEACEIFBHJK, mm)
	w.U8(yaw)                                         // LKDFEKDDFIA  facing yaw (0..255 = 0..360°)
	w.U32(0)                                          // CEDJCPLOLNE  revive/motion init state param
	return w.B
}
