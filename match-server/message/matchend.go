package message

// EMatchEndType reasons (cmd 103 first byte). CSSOTeamEliminate is the canonical CS
// "a team reached the win target / was eliminated" match-over reason.
const MatchEndCSSOTeamEliminate byte = 4

// MatchEnd builds the cmd 103 (RUDP_MATCH_END, message BAOOABOAAJJ) payload. The client
// shows the CS result screen, stores MatchEndRank, and calls CurrentGame.OnMatchEnd()
// which tears the match down and returns to the lobby. cmd 103 ALONE ends a CS match
// (cmd 221 is unrelated in this build). rank is PER-CLIENT — 1 for the winning team's
// players, 2 for the losing team's; the WIN vs LOSE banner is derived from the round
// scores we already streamed (PRI field 28) together with this rank. Field order
// verified from BAOOABOAAJJ::UnSerialize @0x37d9808 + handler EPGJDBMDJAA @0x1940eb0.
// See [[bot-networkaipawn]].
func MatchEnd(rank uint16) []byte {
	w := &Writer{}
	w.U8(MatchEndCSSOTeamEliminate) // JMHLAAMIHKD  EMatchEndType (reason)
	w.U16(rank)                     // GLNDFCHKECH  -> MatchEndRank (1=win, 2=lose)
	w.I16(0)                        // HLODHMFICHM  teammates list (count 0)
	w.I16(0)                        // FKMGJHMNMCF  winners list (count 0)
	w.U64(0)                        // LPDLNKFODKD  killer uid
	w.U64(0)                        // PKPENIHCKLN  revenger uid
	w.Bool(true)                    // FBNGPHHIEBH  NeedShowExtraResultBG
	w.U64(0)                        // EHNIDFHPDFO  final-kill killer uid
	w.I32(0)                        // IMDHJBBPFNC  final-kill weapon id
	w.U64(0)                        // NOLIJNPPOME  final-kill victim uid
	return w.B
}
