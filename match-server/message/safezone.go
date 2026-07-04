package message

// SafeZone TimeSpanType (cmd 117 wire byte JHAHMIMOMHN): 0=idle, 1=waiting (circle
// drawn statically at Outer), 2=shrinking (client lerps centre+radius Outer->Inner
// over [StartMs,EndMs]). Only ZoneShrinking interpolates; the HUD timer counts down
// EndMs regardless.
const (
	ZoneIdle      byte = 0
	ZoneWaiting   byte = 1
	ZoneShrinking byte = 2
)

// SafeZoneChange builds the cmd 117 (RUDP_SAFE_ZONE_CHANGE, message KGOHADAMBLI)
// payload for a circular Contra Squad zone. The Fight-phase HUD timer reads
// (EndMs - GameTime*1000)/1000, so EndMs on the client GameTime clock is what makes the
// round timer count down. Centres are world metres ×1000 (int32 mm); radii are whole
// world metres (uint32, NOT ×1000). Verified from KGOHADAMBLI::UnSerialize @0x3730970
// (fields: stageID, OuterCenter, OuterRadius, InnerCenter, InnerRadius, TimeSpanType,
// StartTime, EndTime, QuickPreShrink, IsPhaseRandomCenter) + SafeZone::InitByMessage
// @0x2800b14. See [[bot-networkaipawn]].
func SafeZoneChange(stageID byte, outer Vec3, outerR float64, inner Vec3, innerR float64, timeSpan byte, startMs, endMs uint32) []byte {
	w := &Writer{}
	w.U8(stageID)
	w.I32(int32(outer.X * 1000)) // OuterCenter (IAAEJDDHOKB): x,z in mm, y dropped
	w.I32(int32(outer.Z * 1000))
	w.U32(uint32(outerR)) // OuterRadius: whole metres
	w.I32(int32(inner.X * 1000))
	w.I32(int32(inner.Z * 1000))
	w.U32(uint32(innerR)) // InnerRadius: whole metres
	w.U8(timeSpan)
	w.U32(startMs) // StartTime (ms, client GameTime clock)
	w.U32(endMs)   // EndTime   (ms) — drives the Fight timer
	w.Bool(false)  // QuickPreShrink
	w.Bool(false)  // IsPhaseRandomCenter
	return w.B
}
