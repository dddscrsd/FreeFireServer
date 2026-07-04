package main

import (
	"encoding/binary"
	"fmt"
	"log"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// handle acks (if reliable) and dispatches one decoded packet to its handler. Each
// case is a thin delegate — the actual work lives in the handleXxx methods below.
func (s *session) handle(p *packet.Packet) {
	if packet.IsReliable(p.Cmd, p.SendOption) {
		s.ack(p.SeqID) // stop the client resending
	}
	switch p.Cmd {
	case packet.CmdHello, packet.CmdReconnect: // 1 / 5
		s.handleHello(p)
	case packet.CmdAck: // 2 — client's ACK of our reliable packets; stub does not resend.
	case packet.CmdPing: // 3
		s.handlePing(p)
	case packet.CmdJoinMatchPost: // 440
		s.handleJoinMatchPost(p)
	case packet.CmdCSPurchase: // 408
		s.handleCSPurchase(p)
	case packet.CmdTakeDamage: // 106
		s.handleTakeDamage(p)
	case packet.CmdChangeHeldItem: // 108
		s.handleChangeHeldItem(p)
	case packet.CmdClientPos: // 1001
		s.handleClientPos(p)
	case packet.CmdPlayerQuitReq: // 191
		s.handlePlayerQuit(p)
	default:
		s.handleUnknown(p)
	}
}

// handleHello answers HELLO / reconnect with S2C_Hello_Res. A cmd 5 reconnect is a
// mid-match transport re-handshake: the client is already in the running battle and
// does NOT re-post cmd 440, so the join packets (100/101/130) must NOT be resent —
// but our CS replication stream died with the old session, so resume it.
func (s *session) handleHello(p *packet.Packet) {
	res := packet.BuildHelloRes("libmadoka", 0, 0, false)
	seq := s.sendHelloRes(res)
	log.Printf("[mm-udp] HELLO(cmd=%d) %v -> S2C_Hello_Res (seq=%d %dB)", p.Cmd, s.remote, seq, len(res))
	if p.Cmd == packet.CmdReconnect && !s.syncStarted {
		if s.player.AccountID == 0 {
			s.player = resolvePlayer("") // fallback player (EntityID=1, RepID 1000) — matches the original join
		}
		s.joined = true
		log.Printf("[mm-udp] reconnect mid-match -> resuming CS sync loop (no re-join) %v", s.remote)
		s.startCSSync()
	}
}

// handlePing echoes the ping back unreliably to keep the client's ping timer alive.
func (s *session) handlePing(p *packet.Packet) {
	s.write(&packet.Packet{SendOption: packet.SendUnreliable, Cmd: packet.CmdPing, Flags: 0, Payload: p.Payload})
}

// handleJoinMatchPost handles cmd 440: decode the prepare_token, resolve the player,
// pick a spawn, answer the join (cmd 100/101/130), and start the CS state stream.
//
// NOTE: cmd 110 BattleStart is NOT used for Contra Squad — it triggers
// LoadMPBattleGame(CREATE_NEW_CONN) (leave waiting island), which just makes the CS
// client reconnect without unlocking movement. The GRI (cmd 901) is the movement
// driver; the authoritative spawn is the position in cmd 101 (see choosePlayerSpawn).
func (s *session) handleJoinMatchPost(p *packet.Packet) {
	pl := resolvePlayer(extractPrepareToken(p.Payload))
	s.arena = choosePlayerSpawn(&pl)
	s.player = pl
	s.joined = true

	s.sendDataLog(packet.CmdJoinMatchRes, message.JoinMatchRes(0), "cmd=100 LGIGCGIDOKP result=0")
	s.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(pl),
		fmt.Sprintf("cmd=101 GKBDLJFGGMI acc=%d ent=%d %q", pl.AccountID, pl.EntityID, pl.Name))

	// Spawn the enemy bot: a second PLAYER_JOIN for a fake remote player whose team is
	// the hibyte of its (fixed) entity id, spawned near us. The SAME entity is reused
	// every round (revived/respawned on the round transition), never a new one.
	s.round = 1
	s.botEntity = botEntityID
	s.bot = botPlayer(pl, s.botEntity)
	s.sendDataLog(packet.CmdPlayerJoin, message.PlayerJoin(s.bot),
		fmt.Sprintf("cmd=101 BOT acc=%d ent=%#x %q pos=(%.1f,%.1f,%.1f)",
			s.bot.AccountID, s.bot.EntityID, s.bot.Name,
			s.bot.SpawnPos.X, s.bot.SpawnPos.Y, s.bot.SpawnPos.Z))

	s.sendDataLog(packet.CmdJoinMatchFinished, message.JoinMatchFinished(), "cmd=130 JoinMatchFinished")
	s.startCSSync()
}

// extractPrepareToken pulls the JWT prepare_token out of a cmd 440 payload. The live
// 1.70 struct layout does NOT match the reference offsets, so scanning for the JWT
// header marker is the reliable path; the struct Token field is only a fallback.
func extractPrepareToken(payload []byte) string {
	if tok := message.ExtractJWT(payload); tok != "" {
		log.Printf("[mm-udp] JOIN_MATCH_POST prepare_token found by scan (%dB)", len(tok))
		return tok
	}
	if req, err := message.ParseJoinMatchReq(payload); err == nil && req.Token != "" {
		log.Printf("[mm-udp] JOIN_MATCH_POST prepare_token from struct field (%dB)", len(req.Token))
		return req.Token
	}
	log.Printf("[mm-udp] JOIN_MATCH_POST no prepare_token; payload(%dB)=%x", len(payload), payload)
	return ""
}

// handleClientPos parses the client's per-tick position update (cmd 1001) and stores
// the local player's world position for the SafeZone out-of-zone check. Payload leads
// with [id u32][X i32][Y i32][Z i32], each coord ×1000 (mm); the rest is rotation/state.
func (s *session) handleClientPos(p *packet.Packet) {
	if len(p.Payload) < 16 {
		return
	}
	x := int32(binary.LittleEndian.Uint32(p.Payload[4:]))
	y := int32(binary.LittleEndian.Uint32(p.Payload[8:]))
	z := int32(binary.LittleEndian.Uint32(p.Payload[12:]))
	pos := message.Vec3{X: float64(x) / 1000, Y: float64(y) / 1000, Z: float64(z) / 1000}
	s.hpMu.Lock()
	s.playerPos = pos
	s.hpMu.Unlock()
	if cfg.debugPos {
		log.Printf("[mm-udp] POS cmd=1001 -> (%.1f,%.1f,%.1f)", pos.X, pos.Y, pos.Z)
	}
}

// handlePlayerQuit handles cmd 191 (RUDP_PLAYER_QUIT_REQ): acknowledge with the quit
// result (cmd 192) and stop the CS sync loop so the server stops streaming GRI/PRI to
// a client that has left the match.
func (s *session) handlePlayerQuit(p *packet.Packet) {
	s.stopped.Store(true)
	s.sendDataLog(packet.CmdPlayerQuitRes, message.PlayerQuit(1), fmt.Sprintf("cmd=192 PlayerQuit payload=%x", p.Payload))
}

// handleUnknown logs an unrecognised packet with its full payload for RE.
func (s *session) handleUnknown(p *packet.Packet) {
	if p.Cmd == packet.CmdSyncServerTime {
		return
	}
	log.Printf("[mm-udp] %v UNHANDLED cmd=%d so=%d seq=%d order=%d flags=%d len=%d payload=%x",
		s.remote, p.Cmd, p.SendOption, p.SeqID, p.OrderID, p.Flags, len(p.Payload), p.Payload)
}
