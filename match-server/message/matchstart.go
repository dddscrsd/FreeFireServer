package message

// Tail of the join sequence, mirroring the reference server (original_to_read/udp.py):
// after JoinMatch_Res (cmd 100) + PlayerJoin (cmd 101), the server sends
// JoinMatchFinished (cmd 130 — "HUD appears / join complete") and then
// BattleStart (cmd 110 — actually starts the match, pulls the client off the
// loading screen).

// JoinMatchFinished builds message::LGBENPIAFIN (cmd 130).
// Reference (v1.24/1.43) had an empty payload; our 1.70 build reads 3 bools.
func JoinMatchFinished() []byte {
	w := &Writer{}
	w.Bool(false) // KIOJFOFBOOL
	w.Bool(false) // IJCHICAIHJJ
	w.Bool(false) // KNEKPOMMLDG
	return w.B
}

// BattleStart builds message::JAKKPBMHLAI (cmd 110):
//   string key | u64 roomID | u64 battleID
// Reference key = "battle_start_%03d" % match_id.
func BattleStart(key string, roomID, battleID uint64) []byte {
	w := &Writer{}
	w.Str(key)      // BNDLDFLHGIF
	w.U64(roomID)   // DGCJANAMDBJ
	w.U64(battleID) // FBDKEAHAEGO
	return w.B
}
