package message

// ZoneEntry is one player's packed zone/team byte for the cmd 457 zone-index message.
type ZoneEntry struct {
	UID    uint32 // player's PlayerID (== our entity id: team<<24 | index)
	Packed byte   // bits0-3 = ZoneID (0-11), bit4 = special-zone, bits5-7 = teamIndex
}

// CSZoneIndex builds the cmd 457 (0x1C9, message GELOHPLLLHA) payload — the per-player zone index that
// drives the CS round-intro CITY NAME. The client stores each entry in m_CSSOPlayerZoneIndexDict keyed
// by UID; the round-start UI (UIHudCSSORoundStartController::SetGameZoneInfo @0x1c638ac) looks up the
// LOCAL player's zone and resolves it via GDADEBKBCOI::EEAKNPJIAJJ(zoneID) @0x1af7ab4 -> the
// {MapID}_GameZoneInfo CSV name loc key (protocol/gamezoneinfodec.txt, ZoneID 0-11). The dict defaults
// to 99 (a miss -> blank/wrong name), so this is what actually sets the city — the ShowCase cmd-294
// path did nothing. Send RELIABLE each round, and INCLUDE the local player's UID. Handler
// JPFHEOJDODC @0x197250c. Wire (LE): [i16 count] then [u32 uid][u8 packed] per entry.
func CSZoneIndex(entries []ZoneEntry) []byte {
	w := &Writer{}
	w.U16(uint16(len(entries)))
	for _, e := range entries {
		w.U32(e.UID)
		w.U8(e.Packed)
	}
	return w.B
}

// PackZone builds the cmd-457 packed byte: zone id (0-11) in the low nibble + team index in bits 5-7
// (bit 4 = special-zone flag, left 0). Client splits it: zone = b&0x0F, team = b>>5.
func PackZone(zoneID, teamIndex byte) byte {
	return (teamIndex << 5) | (zoneID & 0x0F)
}
