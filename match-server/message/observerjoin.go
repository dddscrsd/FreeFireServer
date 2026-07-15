package message

// ObserverJoin builds message::ILIKAAODPKH — cmd 230 RUDP_OBSERVER_JOIN with observer type
// EObserverType_Replay (2). This is the game's OWN replay-seed packet: on a LIVE client it is a
// complete NO-OP (handler NGNCJFLDMAP @0x1934d78 runs the observer setup OCCNGJLAIPB only inside its
// IsReplayState branch — for type 2 with IsReplayState false it just logs and returns; no observer
// flag, no camera switch, no CloseMask), but in a REPLAY (IsReplayState true) it establishes the
// observed pawn + fires the ADD_OBSERVER event -> UIInGameScene::OnAddObserver -> CloseMask
// UNCONDITIONALLY. That is how a replay dismisses its loading mask: the normal streamer-finished gate
// can never be satisfied in a replay (there is no local player — Player::IsLocalPlayer returns false
// in replay state — so the world-streamer never resolves a focus). We reseed this after the cmd-101
// roster so the observed entity already exists in the recording.
//
// Wire (Serialize @0x369ba78 + sub-message IICBMDHGPEK::Serialize @0x369a3d4, positional little-endian,
// no length prefixes — same convention as cmd 101 / cmd 149), 24 bytes:
//
//	IICBMDHGPEK (FNCMBMMKLLI sub-message, inlined):
//	  u64 MIJOCMKONAD   account id (any nonzero; keys the observer dict)
//	  u32 IHAAMHPPLMG   the OBSERVER's entity id (the recorder's own pawn)
//	  u32 GHHENFFJPHK   the OBSERVED target entity id (== self: the recorder watches its own pawn)
//	u32 FBHGGMDGIJP   observer type = 2 (EObserverType_Replay) -- the load-bearing value
//	u32 CEDJCPLOLNE   0
func ObserverJoin(accountID uint64, entity uint32) []byte {
	w := &Writer{}
	w.U64(accountID) // MIJOCMKONAD
	w.U32(entity)    // IHAAMHPPLMG  (observer = the recorder's own pawn)
	w.U32(entity)    // GHHENFFJPHK  (observed target = self)
	w.U32(2)         // FBHGGMDGIJP = EObserverType_Replay (live no-op; replay -> observer + CloseMask)
	w.U32(0)         // CEDJCPLOLNE
	return w.B
}
