package message

import "encoding/binary"

// Emote play/sync (cmd 0x171 = 369, bidirectional). RE-confirmed for FF 1.70.1: the client plays an emote
// locally then sends LAPCIFKMAAP; the server stamps the emoter's playerId and rebroadcasts NIBOCOJJCCJ so
// every OTHER client plays the same emote on that remote pawn (PlayerNetwork::SyncPlayEmotionAnimation ->
// Player::PlayEmotionAnimation). emoteId is the equipped emote's CollectionEmote.csv DataID (e.g.
// 909000001) — passed through unchanged. See [[cs-emotes]].

// EmoteReq is a parsed cmd 369 C2S (message LAPCIFKMAAP): the client played an emote. Wire (15B LE,
// UnSerialize @0x3735c78): [u32 EmoteID][u8 EffIdx][u8 IsLooping][u8 IsLeadIngame][u32 FollowLeaderId]
// [u32 LeadSimTick]. The lead/follow fields drive group dance-party sync and are ignored for basic play.
type EmoteReq struct {
	EmoteID   uint32
	EffIdx    byte
	IsLooping byte
}

// ParseEmoteReq decodes a cmd 369 C2S payload; ok=false if it lacks the emote id + the two anim bytes.
func ParseEmoteReq(payload []byte) (EmoteReq, bool) {
	if len(payload) < 6 {
		return EmoteReq{}, false
	}
	return EmoteReq{
		EmoteID:   binary.LittleEndian.Uint32(payload[0:]),
		EffIdx:    payload[4],
		IsLooping: payload[5],
	}, true
}

// EmoteBroadcast builds the cmd 369 S2C payload (message NIBOCOJJCCJ) relayed to the other players so they
// play the emoter's emote on its remote pawn. Wire (10B LE, UnSerialize @0x34970c0): [u32 PlayerID]
// [u32 EmoteID][u8 EffIdx][u8 IsLooping].
func EmoteBroadcast(playerID, emoteID uint32, effIdx, isLooping byte) []byte {
	w := &Writer{}
	w.U32(playerID)
	w.U32(emoteID)
	w.U8(effIdx)
	w.U8(isLooping)
	return w.B
}
