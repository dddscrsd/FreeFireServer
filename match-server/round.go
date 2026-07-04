package main

import (
	"fmt"
	"log"
	"math"
	"time"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// localTeamID / enemyTeamID are the team ids our cmd 409 reports as the round winner:
// the client derives VICTORY vs DEFEAT by comparing the local player's team (hibyte).
// Local win -> localTeamID (VICTORY); the local player dying to the zone -> enemyTeamID
// (DEFEAT). See [[bot-networkaipawn]].
const (
	localTeamID = 1 // hibyte of playerEntityID (0x01000001)
	enemyTeamID = 2 // hibyte of botEntityID (0x02000002)
)

// Round-transition hold durations. The client hardcodes the flow off the Post phase:
// banner ~5s, then it auto-plays FadeThenToLight (fade-to-black ~1s -> hold-black ~2.5s
// -> fade-to-light ~0.1s), so its own fade-back-to-light fires ~8.5s into Post. We must
// reposition during the black window (~6-8s) AND switch to Introduction BEFORE that
// auto fade-to-light — otherwise the screen fades to light twice (Post's auto one, then
// Introduction's). So: hold Post ~6.7s total (black by ~6s), then Introduction ~1.3s.
const (
	postToBlack   = 6300 * time.Millisecond // banner (~5s) + fade-to-black (~1s) — screen is black after this
	postBlackHold = 400 * time.Millisecond  // brief black beat for the reposition/teleport to land
	introReveal   = 1300 * time.Millisecond // Introduction fires here (before Post's auto to-light) — single fade
)

// roundResult sends the cmd 409 round-result crediting winnerTeam — this raises the
// VICTORY/DEFEAT banner and (with the field-28 PRI score) the scoreboard.
func (s *session) roundResult(winnerTeam byte) {
	s.sendDataLog(packet.CmdCSRoundResult, message.CSRoundResult(winnerTeam, playerEntityID),
		fmt.Sprintf("cmd=409 round result: team %d wins round %d (score %d-%d)",
			winnerTeam, s.round, s.teamScore[0], s.teamScore[1]))
}

// matchEnd ends the match with cmd 103 (MatchEnd): rank 1 when the local team won, 2
// when it lost. This shows the CS result screen and makes the client tear down and leave
// the match. Sent on the deciding round INSTEAD of the round-result (cmd 409); the
// win/lose banner comes from the streamed scores + this rank.
func (s *session) matchEnd(localWon bool) {
	rank, result := uint16(2), "LOSE"
	if localWon {
		rank, result = 1, "WIN"
	}
	s.sendDataLog(packet.CmdMatchEnd, message.MatchEnd(rank),
		fmt.Sprintf("cmd=103 MatchEnd rank=%d local=%s final=%d-%d",
			rank, result, s.teamScore[0], s.teamScore[1]))
}

// roundTransition runs the real between-rounds flow (RE'd via IDA): the GRI Post phase
// holds the VICTORY banner ~5s then the client auto-fades to black; during the black
// window we teleport the local player and respawn the SAME bot at the new arena; the
// Introduction phase reveals the new round and fades back to light. The csSyncLoop then
// opens the next buy phase.
func (s *session) roundTransition(revivePlayer bool) {
	// Pick the next arena now so spawn targets are ready before the black window.
	s.arena = pickArena()
	s.player.SpawnPos, s.player.SpawnFace = s.arena.spawnFor(localFaction)
	s.bot = botPlayer(s.player, s.botEntity) // same entity, new spot near the player
	log.Printf("[mm-udp] round transition -> arena=%q local=(%.1f,%.1f,%.1f) revivePlayer=%v %v",
		s.arena.City, s.player.SpawnPos.X, s.player.SpawnPos.Y, s.player.SpawnPos.Z, revivePlayer, s.remote)

	// Post: banner then the client's hardcoded fade-to-black. Hold until fully black.
	if !s.streamPhase(message.CSPhasePost, postToBlack, nil, false) {
		return
	}
	// Fully black now — reposition unseen. Reset both to full HP; respawn the bot.
	s.hpMu.Lock()
	s.hp = map[uint32]uint16{playerEntityID: maxHP, s.botEntity: maxHP}
	s.playerPos = s.player.SpawnPos // reset to the new spawn so the shrinking zone can't
	// damage a STALE position (cmd 1001 is throttled while the player stands still, so
	// without this the zone check keeps using the previous round's city coords)
	s.hpMu.Unlock()
	s.respawnBot()
	// Reposition the local player at the new gate. A DEAD player is stuck spectating, so
	// re-join it via cmd 101 (authoritative — the client accepts it and revives the pawn).
	// An ALIVE (won) player can't be re-joined (the client IGNORES cmd 101 for a live
	// local player), so stream the force-teleport instead — but ONLY inside this
	// black/transition window (the player is frozen here so it sticks), NOT into the buy
	// phase, so free-roam works there.
	var pull *message.Vec3
	if revivePlayer {
		// Respawn the dead local player via cmd 388 (NOTIFYREVIVE) — RE-proven the ONLY
		// packet in the build that clears a Player's dead flag (StopPendingRevive ->
		// set_IsDead(false)). It reuses the EXISTING player object (no cmd 101 re-add, so
		// no team glitch — the enemy bot is never touched) and repositions + un-spectates
		// the client. The full un-spectate only fires because the death left the observer
		// on the player itself (see spectateTarget). HP stays on the intact PRI binding;
		// re-give the loadout the death dropped. cmd 388 carries the spawn, so no pull.
		s.respawnLocalPlayer()
		time.Sleep(150 * time.Millisecond)
		s.bindPRIs()         // re-bind so the streamed PRI HP lands cleanly on the revived pawn
		s.giveLoadout(false) // death dropped the weapon to fists — re-give the USP, keep coins
	} else {
		pull = &s.player.SpawnPos
		s.reissueLoadout() // alive: refill every kept weapon's magazine + reserve for the new round
	}
	if !s.streamPhase(message.CSPhasePost, postBlackHold, pull, false) {
		return
	}
	// Introduction: reveal the new round number and fade back to light (still frozen, so
	// keep pulling an alive player to the new gate through the fade).
	s.round++
	s.broadcastZone() // re-draw the safe zone at the NEW city now that the player teleported
	time.Sleep(150 * time.Millisecond)
	if !s.streamPhase(message.CSPhaseIntroduction, introReveal, pull, false) {
		return
	}
}

// respawnLocalPlayer respawns the dead local player via cmd 388 (NOTIFYREVIVE) at the new
// gate. It is the ONLY packet that clears a Player's dead flag; it reuses the existing
// player object (no team re-add, unlike cmd 101), repositions it, and returns the camera
// to first person. The un-spectate requires the death to have left the observer on the
// player itself (see spectateTarget). HP comes from the still-bound PRI stream.
func (s *session) respawnLocalPlayer() {
	yaw := yawByte(s.player.SpawnFace)
	s.sendDataLog(packet.CmdNotifyRevive, message.NotifyRevive(playerEntityID, s.player.SpawnPos, yaw),
		fmt.Sprintf("cmd=388 local player revive ent=%#x pos=(%.1f,%.1f,%.1f) yaw=%d",
			uint32(playerEntityID), s.player.SpawnPos.X, s.player.SpawnPos.Y, s.player.SpawnPos.Z, yaw))
}

// yawByte encodes a horizontal facing vector as the cmd 388 yaw byte (0..255 = 0..360°,
// measured from +Z toward +X — the client's AngleAxis(up) convention).
func yawByte(face message.Vec3) byte {
	deg := math.Atan2(face.X, face.Z) * 180 / math.Pi
	if deg < 0 {
		deg += 360
	}
	return byte(deg / 360 * 255)
}

// respawnBot brings the SAME bot entity back for the new round: re-send its PLAYER_JOIN
// at the new position (re-materialising the pawn the death packet removed) and teleport
// it — the entity id and RepID stay stable (no per-round new entity).
func (s *session) respawnBot() {
	s.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(s.bot),
		fmt.Sprintf("cmd=101 BOT respawn (same ent=%#x) pos=(%.1f,%.1f,%.1f)",
			s.botEntity, s.bot.SpawnPos.X, s.bot.SpawnPos.Y, s.bot.SpawnPos.Z))
	s.tpSeq++
	s.sendData(packet.CmdTeleport, message.ForceTeleport(s.botEntity, s.tpSeq, s.bot.SpawnPos, s.bot.SpawnFace, 0))
}
