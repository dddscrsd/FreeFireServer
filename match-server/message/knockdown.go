package message

// Knockdown / DBNO wire (FF 1.70.1 Contra Squad): a player reduced to 0 HP on a team that still has
// >1 member alive is KNOCKED DOWN (crawling + bleeding, still ALIVE), revivable mid-round by a
// teammate — distinct from the cmd-107 death and the cmd-388 pending-revive round-reset. See memory
// cs-knockdown-revive for the full wire RE.

// KnockDown builds cmd 139 RUDP_KNOCK_DOWN (message NKDBFGLPCCF): tells the client that `victim` was
// knocked down by `knocker`. The client puts the pawn into the crawl/bleed pose (NOT set_IsDead).
// Payload (LE): victim u32, knocker u32, weaponData i32, bodyPart u8, timestamp u64 (unused here),
// weaponSkin u32, headshot bool. weaponData is written as its u32 bit pattern (a positive weapon id).
func KnockDown(victim, knocker, weaponData, weaponSkin uint32, bodyPart uint8, headshot bool) []byte {
	w := &Writer{}
	w.U32(victim)
	w.U32(knocker)
	w.U32(weaponData)
	w.U8(bodyPart)
	w.U64(0)
	w.U32(weaponSkin)
	w.Bool(headshot)
	return w.B
}

// KnockRevive builds cmd 140 RUDP_KNOCK_REVIVE (message DJOOMNBJMOG): a mid-round teammate revive
// completed — un-knock `revived`, rescued by `rescuer`. Payload (LE): revived u32, rescuer u32.
func KnockRevive(revived, rescuer uint32) []byte {
	w := &Writer{}
	w.U32(revived)
	w.U32(rescuer)
	return w.B
}

// RescueProgress builds the DCBAMPDIHIG rescue ack/progress — sent on cmd 142, the SAME opcode the
// client used for START_RESCURE (bidirectional). success=true accepts the rescue. The client drives
// the RESCUER's progress bar (over endMs-startMs) off the first u32, and the RESCUED player's own
// "being revived" indicator (SyncBeRescured -> Player.IsRescured -> UIHUDHELPING) off the SECOND u32
// — so the id order is (rescuer, rescued), NOT (rescued, rescuer). resultCode 0 = ok. Payload (LE):
// success bool, rescuerId u32, rescuedId u32, startMs u64, endMs u64, resultCode u32.
func RescueProgress(success bool, rescuer, rescued uint32, startMs, endMs uint64, resultCode uint32) []byte {
	w := &Writer{}
	w.Bool(success)
	w.U32(rescuer) // 1st u32 = RESCUER (drives the rescuer's progress bar)
	w.U32(rescued) // 2nd u32 = RESCUED/downed (drives the downed player's "being revived" indicator)
	w.U64(startMs)
	w.U64(endMs)
	w.U32(resultCode)
	return w.B
}
