package message

// CSRoundResult builds the cmd 409 (RUDP_CS_CURROUND_RESULT, message DMJPAJFMMMB)
// payload. The client's handler (KPDMJKOEHEE::EKPPFGMAGNJ @0x196b514) reads NCHOPFNDDNM
// as the WINNING TEAM id and calls UpdateWinTeamIdCurRound — the client tallies the
// per-team scoreboard from these across rounds and derives VICTORY vs DEFEAT by
// comparing the local player's team. The bonus list + MVP (NDECJGIMIFA) only populate
// the result panel. Field order verified from DMJPAJFMMMB::UnSerialize @0x36cbb58; we
// send an empty bonus list.
func CSRoundResult(winnerTeam byte, mvpPlayer uint32) []byte {
	w := &Writer{}
	w.U8(winnerTeam) // NCHOPFNDDNM: winning team id -> VICTORY/DEFEAT banner + score tally
	w.I16(0)         // DELEJFIFDCL: round-bonus list (OLAHGOOEKDB), empty
	w.U8(0)          // EHDNIOLBBDD
	w.U32(mvpPlayer) // NDECJGIMIFA: MVP player id (shown on the result panel)
	return w.B
}
