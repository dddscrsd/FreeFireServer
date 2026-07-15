package message

// ShowDamage builds message::KGMCGDGKCDL — cmd 168 RUDP_SHOW_DAMAGE, the floating damage-number
// event. The shooter's own client renders its numbers locally; we relay this to a SPECTATOR watching
// the shooter so the dead teammate sees the same numbers. Handler KPDMJKOEHEE::HDJNMCLHFOC @0x194852c
// dispatches EventID_LOCALPLAYER_HIT_OTHERS (309) — the same event the shooter fires from
// Player::TakeDamage — driving the damage-number UI. It suppresses ONLY when the shooter is the local
// player (CIILMLJIDBJ && CurrentLocalPlayerID == attacker), so a spectator (localId != attacker)
// always shows the number.
//
// Wire (UnSerialize @0x3730110, little-endian, 26 bytes):
//
//	u32  attacker    (LIIGLCNGOHG — the suppress gate compares this to the local player id)
//	u32  victim      (IBPJGEDFFAA — the number floats above this pawn)
//	u16  damage      (BJBPPEBIPFA — the value rendered)
//	u32  hitRegion   (GJJOGIKHBFB — DBNMCJLEFJI head/body enum; drives the number COLOR)
//	i32  weapon      (PIAMIOFEBKF)
//	bool             (LPKDILABMLK)
//	i32              (NEHAKPPGLNL)
//	bool suppressOwn (CIILMLJIDBJ — skip on the SHOOTER's own screen only)
//	bool             (KOKPPKDLONF)
//	bool             (EGKHIDGBAEL)
//
// suppressOwn is TRUE so we can send this to the SHOOTER itself: the shooter renders its own damage
// numbers locally, so the handler suppresses this duplicate (gate: suppressOwn && localId==attacker)
// — but it still lands in the shooter's RECORDING, and the handler's replay branch (IsReplayState)
// dispatches the number UNCONDITIONALLY, so the replay shows the recorder's own damage. A spectator
// (localId != attacker) is never suppressed, so they see it too. See deathmatch.go applyDamage.
//
// headshot drives the number COLOR: both the cmd-168 handler (HDJNMCLHFOC) and Player::TakeDamage feed
// GJJOGIKHBFB into LOCALPLAYER_HIT_OTHERS arg[3] as the DBNMCJLEFJI enum, whose value 1 is the isHeadshot
// flag (the same KENBMOOEHBG==1 that gates the headshot OnKill/RecordDamageEnemy path) → the client
// renders a RED critical number. Anything else (0) is a default YELLOW number. Without this a spectator's
// / replay's headshot damage always rendered yellow because the shooter's OWN local render (which knows
// the body part) never reached them — only this server cmd-168 does.
func ShowDamage(attacker, victim uint32, damage uint16, weapon uint32, headshot bool) []byte {
	hitRegion := uint32(0) // DBNMCJLEFJI default (body/limb) — yellow number
	if headshot {
		hitRegion = 1 // DBNMCJLEFJI headshot — red critical number
	}
	w := &Writer{}
	w.U32(attacker)      // LIIGLCNGOHG
	w.U32(victim)        // IBPJGEDFFAA
	w.U16(damage)        // BJBPPEBIPFA
	w.U32(hitRegion)     // GJJOGIKHBFB — hit-region enum (1 = headshot/red, 0 = default/yellow)
	w.I32(int32(weapon)) // PIAMIOFEBKF
	w.Bool(false)        // LPKDILABMLK
	w.I32(0)             // NEHAKPPGLNL
	w.Bool(true)         // CIILMLJIDBJ — suppress on the shooter's OWN live screen (dupe of its local render); recorded + shown in replay
	w.Bool(false)        // KOKPPKDLONF
	w.Bool(false)        // EGKHIDGBAEL
	return w.B
}
