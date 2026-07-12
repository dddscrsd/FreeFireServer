package message

import "encoding/binary"

// MoveEntry is one player's state in a movement-relay batch: the u8 slot the client keys its
// Dictionary<byte,Player> on (the entity-id low byte) plus the 37-byte state body — which is
// byte-identical to the upstream cmd-1001 body [4:41], so the relay just re-frames it.
type MoveEntry struct {
	Slot byte
	Body []byte // 37 bytes: pos+moveDir+targetDir+velocity+moveState+flag+aimDir+platform
}

// MoveBatch builds the cmd 1001 downstream batch broadcast (unreliable) to every client:
//
//	[i32 matchTime][i16 count][count× (u8 slot + 37B body)][u8 skipOwn][u32 tickSeq]
//
// The SAME batch (every player, incl. the sender) fans to everyone. The skip-own byte is TRUTHY-means-
// skip (IDA CEJMAIHOFMM apply @0x1936184): with skipOwn=1 each LIVE client drops the entry whose slot
// == its own local slot (its entity-id low byte) and applies only the others — so streaming a sender
// its OWN entry back doesn't rubber-band it (its pawn stays input-driven). In a REPLAY the apply forces
// skip-own OFF (IsReplayState), so the recorder's own entry IS applied and its pawn moves on playback —
// which is exactly why we self-echo (see match.go streamMovement). tickSeq must strictly increase (the
// client's per-player seq gate drops a stale/equal seq). Fixed little-endian.
// ⚠️ skipOwn is 1 (NOT 0): 0 means "apply own", which WOULD rubber-band the local player now that we
// echo its own entry back. The reference udp.py targets a DIFFERENT client version (u32 count, no skip-
// own byte) — that layout desyncs the entry stream on 1.70.1; do NOT use it here. See cs-movement-sync.
func MoveBatch(matchTime int32, tickSeq uint32, entries []MoveEntry) []byte {
	buf := make([]byte, 0, 11+38*len(entries))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(matchTime))    // i32 matchTime
	buf = binary.LittleEndian.AppendUint16(buf, uint16(len(entries))) // i16 count
	for _, e := range entries {
		buf = append(buf, e.Slot)
		buf = append(buf, e.Body...)
	}
	buf = append(buf, 1)                                 // skipOwn=1: LIVE client drops its OWN slot; REPLAY forces it off (applies own) -> recorder moves
	buf = binary.LittleEndian.AppendUint32(buf, tickSeq) // u32 tickSeq
	return buf
}

// MoveEntryV2 is one player's state in a cmd-1012 (UDP_PLAYER_STATE_SYNC_V2) batch — the compressed
// 25-byte form that FF 1.70.1 actually applies to remote pawns (the cmd-1001 downstream is ignored
// on this build). Pos + Vel copy verbatim from the upstream core (same encodings); the two rotation
// vectors collapse to three quantized angle bytes (byte = deg × 255/360; 0 = a default facing).
type MoveEntryV2 struct {
	Slot                      byte
	Pos                       []byte // 12 bytes: 3× i32 world pos ×1000
	Vel                       []byte // 6 bytes: 3× i16 velocity ×100
	BodyYaw, AimYaw, AimPitch byte   // quantized facing/aim angles
	MoveState, Flag, Platform byte
}

// MoveBatchV2 builds the cmd 1012 UDP_PLAYER_STATE_SYNC_V2 downstream batch:
//
//	[i32 matchTime][i16 count][count× 25B entry][u32 seq]
//
// No skip-own byte — V2 excludes the local player by a client-side type check. seq must strictly
// increase (per-player seq gate drops stale/equal). Fixed little-endian. See cs-movement-sync.
func MoveBatchV2(matchTime int32, seq uint32, entries []MoveEntryV2) []byte {
	buf := make([]byte, 0, 10+25*len(entries))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(matchTime))
	buf = binary.LittleEndian.AppendUint16(buf, uint16(len(entries)))
	for _, e := range entries {
		buf = append(buf, e.Slot)
		buf = append(buf, e.Pos...)                        // 12
		buf = append(buf, e.Vel...)                        // 6
		buf = append(buf, e.BodyYaw, e.AimYaw, e.AimPitch) // 3
		buf = append(buf, e.MoveState, e.Flag, e.Platform) // 3
	}
	buf = binary.LittleEndian.AppendUint32(buf, seq)
	return buf
}
