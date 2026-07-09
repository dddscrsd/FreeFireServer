package message

import "encoding/binary"

// Named battle-flag world object, planted by the flag emotes (909000034 EMOTE_BATTLEFLAG, 909000128
// EMOTE_UNIONWAR). Upstream cmd 0x114 (message GILPGGDOOGE, RequestUseBattleFlag) is the client's plant
// request; downstream cmd 0x115 (message AACLJKNELJE) is the server's authoritative spawn/despawn broadcast.
// The flag renders the owner's NAME overhead, resolved CLIENT-SIDE from ownerPlayerId — never sent as text.
// See [[cs-emotes]].

// BattleFlagReq is a parsed cmd 0x114 C2S (message GILPGGDOOGE): the client planted a flag. Wire (20B LE,
// Serialize @0x3688480): [u32 owner][i32 x][i32 y][i32 z (each ×1000 mm)][u32 flagConfigId]. flagConfigId
// is the player's equipped battle-flag cosmetic iID — the server echoes it back unchanged.
type BattleFlagReq struct {
	Owner    uint32
	Pos      Vec3
	ConfigId uint32
}

// ParseBattleFlagReq decodes a cmd 0x114 payload; ok=false if it's too short.
func ParseBattleFlagReq(payload []byte) (BattleFlagReq, bool) {
	if len(payload) < 20 {
		return BattleFlagReq{}, false
	}
	return BattleFlagReq{
		Owner: binary.LittleEndian.Uint32(payload[0:]),
		Pos: Vec3{
			X: float64(int32(binary.LittleEndian.Uint32(payload[4:]))) / 1000,
			Y: float64(int32(binary.LittleEndian.Uint32(payload[8:]))) / 1000,
			Z: float64(int32(binary.LittleEndian.Uint32(payload[12:]))) / 1000,
		},
		ConfigId: binary.LittleEndian.Uint32(payload[16:]),
	}, true
}

// BattleFlagEntry is one AMPGELLIDGH: spawn (Active) or despawn (!Active) one flag object.
type BattleFlagEntry struct {
	Owner    uint32
	ObjectId uint32
	Active   bool
	Pos      Vec3
	ConfigId uint32
}

// BattleFlagSync builds the cmd 0x115 payload (message AACLJKNELJE): a count-prefixed list of flag
// spawn/despawn entries — sending old(active=0)+new(active=1) in one list replaces a player's flag
// atomically. Per entry (AMPGELLIDGH, 26B LE, UnSerialize @0x37d5bc0): [u32 owner][u32 objectId]
// [u8 active][i32 x][i32 y][i32 z (each ×1000 mm)][u32 flagConfigId][u8 extra=0].
func BattleFlagSync(entries []BattleFlagEntry) []byte {
	w := &Writer{}
	w.I16(int16(len(entries)))
	for _, e := range entries {
		w.U32(e.Owner)
		w.U32(e.ObjectId)
		w.Bool(e.Active)
		w.I32(int32(e.Pos.X * 1000))
		w.I32(int32(e.Pos.Y * 1000))
		w.I32(int32(e.Pos.Z * 1000))
		w.U32(e.ConfigId)
		w.U8(0) // trailing byte — server-set, not read by SyncAction, default 0
	}
	return w.B
}
