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
	// MVP display id: use the primary human's REAL entity (== playerEntityID for a team-1-first match,
	// but a valid team-2 entity when a room put a team-2 player in first) rather than the fixed const.
	s.match.broadcastData(packet.CmdCSRoundResult, message.CSRoundResult(winnerTeam, s.entityID),
		fmt.Sprintf("cmd=409 round result: team %d wins round %d (score %d-%d)",
			winnerTeam, s.match.round, s.match.teamScore[0], s.match.teamScore[1]))
}

// matchEnd ends the match with cmd 103 (MatchEnd), sent PER PLAYER: rank 1 to the WINNING
// team's members, 2 to the losers. This shows the CS result screen and makes each client tear
// down and leave. Sent on the deciding round INSTEAD of the round-result (cmd 409); the win/lose
// banner comes from the streamed scores + this rank. A single broadcast rank was the bug — it
// gave the LOCAL team's perspective to everyone, so when the local team lost BOTH sides saw
// rank=2 (LOSE) even though the other team had won.
func (m *Match) matchEnd() {
	for _, p := range m.players {
		rank, result := uint16(2), "LOSE"
		if p.team == m.winnerTeam {
			rank, result = 1, "WIN"
		}
		p.sendDataLog(packet.CmdMatchEnd, message.MatchEnd(rank),
			fmt.Sprintf("cmd=103 MatchEnd rank=%d %s ent=%#x final=%d-%d",
				rank, result, p.entityID, m.teamScore[0], m.teamScore[1]))
	}
	m.publishResult() // durable settlement (match.result / match.ended) to the bus, if enabled
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
	m := s.match
	m.broadcastData(packet.CmdPlayerJoin, message.PlayerJoin(m.bot, m.serverTick()),
		fmt.Sprintf("cmd=101 BOT respawn (same ent=%#x) pos=(%.1f,%.1f,%.1f)",
			m.botEntity, m.bot.SpawnPos.X, m.bot.SpawnPos.Y, m.bot.SpawnPos.Z))
	m.tpSeq++
	m.broadcastData(packet.CmdTeleport, message.ForceTeleport(m.botEntity, m.tpSeq, m.bot.SpawnPos, m.bot.SpawnFace, 0), "")
}
