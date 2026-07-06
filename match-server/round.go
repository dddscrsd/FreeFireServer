package main

import (
	"fmt"
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
			winnerTeam, s.match.round, s.match.teamScore[0], s.match.teamScore[1]))
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
			rank, result, s.match.teamScore[0], s.match.teamScore[1]))
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
	s.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(s.match.bot),
		fmt.Sprintf("cmd=101 BOT respawn (same ent=%#x) pos=(%.1f,%.1f,%.1f)",
			s.match.botEntity, s.match.bot.SpawnPos.X, s.match.bot.SpawnPos.Y, s.match.bot.SpawnPos.Z))
	s.match.tpSeq++
	s.sendData(packet.CmdTeleport, message.ForceTeleport(s.match.botEntity, s.match.tpSeq, s.match.bot.SpawnPos, s.match.bot.SpawnFace, 0))
}
