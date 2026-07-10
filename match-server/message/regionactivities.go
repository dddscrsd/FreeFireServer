package message

// SyncRegionActivities builds message::DKKNMJDABAC (cmd 176 RUDP_SYNC_REGION_ACTIVITIES).
//
// RE'd from the client (FF 1.70.1): serializer DKKNMJDABAC::Serialize @0x36ca1f4, reader
// UnSerialize @0x36ca434, handler NFJPHMKKEBF::MMFFHFLMBHO @0x1a31cd8 (dispatched from cmd 176 via
// KPDMJKOEHEE::FHNOJHNLKAP @0x195233c). FastBinaryWriter, little-endian, ENABLE_FAST_PROTO=0 — the
// same wire mode as every other S2C message (strings = int32 length prefix + UTF-8).
//
// Wire layout:
//
//	i16 count N
//	N × string    -- the region-activity LIST (field LGDEODKDPOI). Each string is parsed
//	                 (GLAJDEODHGD::BIFNNGPADHO) into a RegionActivityData {id,type,style,priority,
//	                 params} and the handler switches on its `type` (0..22).
//	string        -- LLEMJCJICKA  (region style A -> match field 0x218)
//	string        -- GFBEIKLKMKK  (region style B -> match field 0x220)
//	string        -- IOPEHOGINCP  (region style C / billboard -> match field 0x228)
//	u32 f32 bool  -- nested FFICCMDPDOA {BKFMPACNBLJ, JOKDJFPBKFL, FMFLJAADBHG}
//
// What it does: the handler parses the list and, for the matching activity type, sets the match's
// ACTIVE REGION TYPE (NFJPHMKKEBF field 0x210, read back via DHLCABDIHNO). Whether or not the list
// is empty it then dispatches the region-refresh event (EventID 110) and re-runs
// SceneGraphics::RefreshDynamicPrefabMeshVisible (@0x2accb74) — which shows every dynamic prefab
// (jump pads, invisible boxes/objects) whose ShowWithType list contains the active type. So cmd 176
// is REQUIRED on join to trigger that visibility pass; without it the prefabs stay hidden.
type RegionActivityPool struct {
	ID   uint32
	Val  float32
	Flag bool
}

func SyncRegionActivities(activities []string, styleA, styleB, styleC string, pool RegionActivityPool) []byte {
	w := &Writer{}
	w.I16(int16(len(activities))) // FastBinaryWriter::Write(short)
	for _, a := range activities {
		w.Str(a)
	}
	w.Str(styleA) // LLEMJCJICKA
	w.Str(styleB) // GFBEIKLKMKK
	w.Str(styleC) // IOPEHOGINCP
	w.U32(pool.ID)
	w.F32(pool.Val)
	w.Bool(pool.Flag)
	return w.B
}

// SyncRegionActivitiesDefault is the minimal "no events to sync" packet: an EMPTY activity list
// (nothing to parse -> no switch, so no throw risk) + the "DEFAULT" region style + a zeroed
// sub-message. Sent once per join so the client fires its dynamic-prefab visibility refresh and the
// map's jump pads / invisible boxes appear. (If a map ever needs live region events, populate the
// activity list instead.)
func SyncRegionActivitiesDefault() []byte {
	return SyncRegionActivities(nil, "DEFAULT", "", "", RegionActivityPool{})
}
