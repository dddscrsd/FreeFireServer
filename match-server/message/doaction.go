package message

// DoAction builds the cmd 201 (server->client S2C_RUDP_DoAction, message FPCOHGNMLBG) payload. It
// drives an action animation on a REMOTE pawn — the client resolves the pawn by PlayerID and calls
// PlayerNetwork::SyncDoAction, which for ActionType 1 (UseMedkit) plays the cure channel animation
// (PlayCureAnimation: ActionState Start -> CURE_BEGIN, End/Cancel -> CURE_END). Relaying the client's
// own cmd 131/113 does NOT animate remotes — cmd 131 only fires a HUD event and cmd 113 has no client
// receive handler — so cmd 201 is the required lever. Wire (LE), from FPCOHGNMLBG::Serialize @0x3682db4:
//
//	[u32 playerID][u32 actionState][u32 actionType][u32 durationMs][i16 extraCount][u32×extraCount][u8 skipLocal]
//
// skipLocal=1: the acting player's OWN client ignores the message (PlayerID == local player) because it
// already animated locally, so this is safe to broadcast to everyone including the actor. See [[cs-fire-sync]].
func DoAction(playerID, actionState, actionType, durationMs uint32, skipLocal bool) []byte {
	w := &Writer{}
	w.U32(playerID)
	w.U32(actionState)
	w.U32(actionType)
	w.U32(durationMs)
	w.I16(0) // extra-info list (empty for the medkit cure)
	w.Bool(skipLocal)
	return w.B
}

// DoAction ActionState values (CJJDECHGLBK): the channel begins, is aborted, or completes.
const (
	DoActionStart  uint32 = 0
	DoActionCancel uint32 = 1
	DoActionEnd    uint32 = 2
)

// DoAction ActionType values (IPHPGHNJCMB / EPreparationTimerType): the channel kind. Only the medkit
// cure is synced today; the others are listed for reference (RepairArmorKit=11, Inhaler=16, Pacemaker=21).
const (
	ActionUseMedkit uint32 = 1
)
