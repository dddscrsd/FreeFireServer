package message

import "math"

// Force-teleport / force-sync message JIIKBLKJCKM (cmd 145 SyncTeleportInfo /
// cmd 144 SyncStateWithServer), reverse-engineered from FF 1.70.1:
//   JIIKBLKJCKM::Serialize @0x3724a74; position sub-msg DEACEIFBHJK = 3x int32
//   (coord*1000, mm; decode LLOABGDLMGK::AHICKBOEJAL @0x3486294 /1000); direction
//   sub-msg HJLNLDIMIMK = 3x int16 (comp*100; decode IBJEJADAJMN @0x34860bc /100).
//   Handlers: DDBOMMOJHDD @0x194ab9c (cmd 144), OHPEJIKNCMD @0x194adc8 (cmd 145)
//   resolve the target player by id (NFJPHMKKEBF::KAGPBMINBIJ) then set_position /
//   set_rotation on the pawn's CachedTransform. This is authoritative for the LOCAL
//   player too (which is autonomous and owns its transform), so it is the only way
//   to force-place the joining player; the client acks via NotifyServerGotForceSyncState.
//
// 37-byte body, little-endian, no length prefixes:
//   u32 playerID | u32 seq | pos(3x i32 = m*1000) | facing(3x i16 = *100) |
//   vel(3x i16 = *100) | u8 physState | u8 subState | u16 platformID | u8 keepRotation
//
// cmd 145 (SyncTeleportInfo) sets BOTH position and rotation — preferred for spawn
// (we must face the opposite gate). cmd 144 (SyncStateWithServer) is position +
// physics-state (fallback). The exact 144/145 -> method pairing is inferred; confirm
// live (send 145 and check rotation is applied).

// Vec3 is a world position or direction in Unity units (meters for a position).
type Vec3 struct{ X, Y, Z float64 }

func r1000(v float64) int32 { return int32(math.Round(v * 1000)) }
func r100(v float64) int16  { return int16(math.Round(v * 100)) }

// ForceTeleport builds the JIIKBLKJCKM body. facing is the (horizontal) unit vector
// the player should look toward (e.g. toward the opposite gate). physState 0 =
// grounded (verify live so the client doesn't drop into parachute/football state).
// Velocity is zero (stationary spawn); keepRotation is false so facing is applied.
func ForceTeleport(playerID, seq uint32, pos, facing Vec3, physState byte) []byte {
	w := &Writer{}
	w.U32(playerID)                                       // IHAAMHPPLMG target player id
	w.U32(seq)                                            // MAKGDLHLKOG force-sync token
	w.vec3i(r1000(pos.X), r1000(pos.Y), r1000(pos.Z))     // CCIKDFGDBAM = DEACEIFBHJK position
	w.hjln(r100(facing.X), r100(facing.Y), r100(facing.Z)) // PDBOFPMDKMM = HJLNLDIMIMK facing
	w.hjln(0, 0, 0)                                       // LMNBBBCKMGB velocity (0 = stationary)
	w.U8(physState)                                       // GHGCGGOLKIP phys-state enum
	w.U8(0)                                               // KIFHHPBIOHK sub-arg
	w.U16(0)                                              // IABJNDBNDBK move-platform id (0 = world)
	w.U8(0)                                               // EKMACOMDMFL keep-rotation (0 = apply facing)
	return w.B
}
