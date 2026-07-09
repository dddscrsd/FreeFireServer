package main

import (
	"log"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// handleEmote handles cmd 369 (0x171 RUDP play-emote): the client played an emote locally and reports it.
// The server stamps the emoter's playerId and rebroadcasts it to the OTHER players so they see the emote
// animation on that remote pawn (the sender already played it, so it is not echoed). The emote id is the
// CollectionEmote.csv DataID, relayed unchanged. Flag emotes (909000034 Pirate's / 909000128 Ruler's) also
// plant a world flag, but that is driven by a SEPARATE message the client sends at the plant keyframe —
// cmd 0x114 RequestUseBattleFlag (see battleflag.go) — not by inspecting the emote id here. See [[cs-emotes]].
func (s *session) handleEmote(p *packet.Packet) {
	req, ok := message.ParseEmoteReq(p.Payload)
	if !ok {
		log.Printf("[mm-udp] cmd=369 emote req too short (%dB): %x", len(p.Payload), p.Payload)
		return
	}
	s.match.broadcastToOthers(s, packet.CmdEmote,
		message.EmoteBroadcast(s.player.EntityID, req.EmoteID, req.EffIdx, req.IsLooping))
	log.Printf("[mm-udp] cmd=369 emote data=%d effIdx=%d loop=%d ent=%#x -> others", req.EmoteID, req.EffIdx, req.IsLooping, s.entityID)
}
