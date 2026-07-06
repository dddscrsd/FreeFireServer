package main

import (
	"encoding/binary"
	"fmt"
	"log"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// route dispatches one decoded packet to its handler. The ack + the serve()/run() routing are
// done by dispatch; route runs either inline (pre-match) or on run()'s goroutine (via the mailbox).
// Each case is a thin delegate — the actual work lives in the handleXxx methods below.
func (s *session) route(p *packet.Packet) {
	switch p.Cmd {
	case packet.CmdHello, packet.CmdReconnect: // 1 / 5
		s.handleHello(p)
	case packet.CmdAck: // 2 — client's ACK of our reliable packets; stub does not resend.
	case packet.CmdPing: // 3
		s.handlePing(p)
	case packet.CmdJoinMatchPrepare: // 439 — buffers the (bigger) prepare_token chunk
		s.handleJoinMatchPrepare(p)
	case packet.CmdJoinMatchPost: // 440
		s.handleJoinMatchPost(p)
	case packet.CmdCSPurchase: // 408
		s.handleCSPurchase(p)
	case packet.CmdAddIcewall: // 218 — client's gloo-wall PLACE request (bidirectional cmd)
		s.handlePlaceIcewall(p)
	case packet.CmdIcewallTakeDamage: // 219 — a placed gloo wall took damage
		s.handleIcewallDamage(p)
	case packet.CmdTakeDamage: // 106
		s.handleTakeDamage(p)
	case packet.CmdChangeHeldItem: // 108
		s.handleChangeHeldItem(p)
	case packet.CmdPickupInventory: // 111 — pick up a ground-loot item
		s.handlePickup(p)
	case packet.CmdDropInventory: // 112 — drop a loadout weapon to the ground
		s.handleDrop(p)
	case packet.CmdUseInventory: // 113 — finished using a consumable (medkit heal-over-time)
		s.handleUseInventory(p)
	case packet.CmdTryUseInventory: // 131 — started channelling a consumable
		s.handleTryUseInventory(p)
	case packet.CmdEquipAttachment: // 122 — client's manual attachment equip (server auto-maxes)
		s.handleEquipAttachment(p)
	case packet.CmdUnequipAttachment: // 123 — never unequip; attachments stay locked on
		s.handleUnequipAttachment(p)
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
	s.sendHelloRes(res)
	log.Printf("[mm-udp] HELLO(cmd=%d) %v -> S2C_Hello_Res (%dB)", p.Cmd, s.remote, len(res))
	if p.Cmd == packet.CmdReconnect && s.match == nil {
		if s.player.AccountID == 0 {
			s.player = resolvePlayer("") // fallback player (EntityID=1, RepID 1000) — matches the original join
		}
		s.joined = true
		m := newMatch(s)
		matchManager.register(m)
		m.setupMatch(s) // arena / roster / round / bot — no re-join packets (client is already in-match)
		log.Printf("[mm-udp] reconnect mid-match -> solo resume (no re-join) %v", s.remote)
		s.startCSMatch()
		s.enqueue(s.resyncWalls) // redraw lost gloo walls on run()'s goroutine (cmd 220)
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
	s.player = resolvePlayer(s.prepareToken(p.Payload))
	s.joined = true
	matchManager.join(s) // Step 5b: route to a match (existing with room, else new) + admit
}

// handleJoinMatchPrepare handles cmd 439 (JOIN_MATCH_PREPARE): the client sends the
// prepare_token (JWT) here as [u32 totalLen][token chunk] when it is large. A small token
// skips 439 and rides entirely in cmd 440; a token too big for one packet is split, the
// bigger half here and the tail in cmd 440. We buffer this chunk (and its declared total
// length); handleJoinMatchPost reassembles the full token.
func (s *session) handleJoinMatchPrepare(p *packet.Packet) {
	if len(p.Payload) < 4 {
		return
	}
	chunkLen := binary.LittleEndian.Uint32(p.Payload[0:])
	chunk := p.Payload[4:]
	if chunkLen > 0 && int(chunkLen) < len(chunk) { // never keep more than declared
		chunk = chunk[:chunkLen]
	}
	s.prep439 = append([]byte(nil), chunk...)
	log.Printf("[mm-udp] JOIN_MATCH_PREPARE cmd=439 token chunk=%dB (declared %d) %v", len(s.prep439), chunkLen, s.remote)
}

// prepareToken reassembles the prepare_token (JWT) from the buffered cmd 439 chunk and
// the cmd 440 payload. It can arrive wholly in cmd 440 (small token), wholly in cmd 439,
// or split across 439 (bigger chunk) + 440 (tail). The 1.70 cmd 440 struct offsets don't
// line up, so scanning for the fixed JWT header marker is the reliable path.
func (s *session) prepareToken(payload440 []byte) string {
	if len(s.prep439) > 0 {
		tok := message.ExtractJWT(s.prep439) // chunk starts at the JWT header
		if tok == "" {
			tok = string(s.prep439) // no marker (raw token bytes) — take as-is
		}
		// The 439 chunk is the bigger half; when it isn't already a whole JWT the tail (the
		// rest of the signature) rides in cmd 440 as the last length-prefixed base64url
		// field, after the metadata. Append it to complete the token.
		if !message.IsCompleteJWT(tok) {
			tail := message.LastJWTField(payload440)
			tok += tail
			log.Printf("[mm-udp] JOIN prepare_token split: 439(%dB)+440 tail(%dB) -> %dB %v",
				len(s.prep439), len(tail), len(tok), s.remote)
		} else {
			log.Printf("[mm-udp] JOIN prepare_token whole in cmd 439 (%dB) %v", len(tok), s.remote)
		}
		return tok
	}
	if tok := message.ExtractJWT(payload440); tok != "" {
		log.Printf("[mm-udp] JOIN prepare_token found in cmd 440 by scan (%dB) %v", len(tok), s.remote)
		return tok
	}
	if req, err := message.ParseJoinMatchReq(payload440); err == nil && req.Token != "" {
		log.Printf("[mm-udp] JOIN prepare_token from cmd 440 struct field (%dB) %v", len(req.Token), s.remote)
		return req.Token
	}
	log.Printf("[mm-udp] JOIN no prepare_token; cmd440(%dB)=%x %v", len(payload440), payload440, s.remote)
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
	s.playerPos = pos
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
