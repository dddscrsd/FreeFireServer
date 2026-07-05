package message

// GLOO WALL (ice wall) packet family, Contra Squad, reverse-engineered from FF
// 1.70.1. The gloo wall (item DataID 1201, equipped in SlotBuilding=13) is placed
// by throwing/firing it; the client sends a place REQUEST and the server spawns an
// authoritative wall that replicates its own state. All wire layouts below are
// verified from the client Serialize/UnSerialize (imagebase 0).
//
// Command numbers (protocol/udp_cmds.txt):
//   218 RUDP_ADD_ICEWALL         (0xDA)  BIDIRECTIONAL, DIFFERENT class each way:
//        C->S place request  = message::JEGADMHBFKB (Serialize @0x37214fc, 29B)
//        S->C spawn broadcast = message::KACGKFMOHNP (UnSerialize @0x372b73c, 41B
//                                = 40B inner JJIODFAHEOG state + 1B state byte)
//   219 RUDP_ICEWALL_TAKE_DAMAGE (0xDB)  C->S = message::CHDLJFJCPFN (UnSer @0x37e7c50)
//   220 RUDP_RESYNC_ICEWALL      (0xDC)  S->C = message::EKKLNKLGFIM (UnSer @0x36d5470)
//   221 RUDP_REMOVE_ICEWALL      (0xDD)  S->C = message::APPOHELKIIG (UnSer @0x37d83bc)
//
// PLACE -> SPAWN FLOW (client fire handler MFBGOIBMHIB::CEFKGANPJBD @0x2756a54):
// on gloo-wall fire the client computes a world position from its aim, builds a
// JEGADMHBFKB and calls GameFacade::Send(0xDA, msg, so=2, cache=0) — i.e. it SENDS
// cmd 218. (There is a separate RUDP_ICEWALL_FIRE=333, but that is a different
// skill path — the normal gloo wall uses cmd 218.) The server assigns a wall
// entity id + a RepID, spawns via a cmd 218 KACGKFMOHNP broadcast to everyone,
// binds the RepID to the wall entity (cmd 118 BindPRI) and streams the wall's
// state byte on its own PRI (cmd 900). The client's cmd 218 receive handler
// (KPDMJKOEHEE::MHBPEPNHLHO @0x195b9e0) feeds ID/WallType/HP/pos/rot/state/RepID/
// flags into the level-object manager to instantiate a LevelIceWall. A fresh
// wall's HP default is 200 and the local max-wall count default is 3 (both read
// straight out of the fire handler).

import "encoding/binary"

// Quat is a facing quaternion. On the wire (message::GAKIHFPIOEF) each component
// is an int16 = component*100 (decode LLOABGDLMGK::EIIAILGOAHP @0x3486488 /100).
type Quat struct{ X, Y, Z, W float64 }

// iceWallState writes one message::JJIODFAHEOG ("PSyncedIceWallStates"), the 40-byte
// element shared by the spawn (218) and resync (220). Field order == the client's
// JJIODFAHEOG::Serialize @0x37257e4:
//
//	u32 ID             (POBGKMDJMDC)  wall entity/game id
//	i32 WallType       (LGBODPJOONM)  building/skin variant (echo the request)
//	i32 HP             (IIPEKJMAEOL)  durability; fresh wall = 200
//	pos DEACEIFBHJK    (i32 x,y,z = m*1000)  world position (CCIKDFGDBAM)
//	rot GAKIHFPIOEF    (i16 x,y,z,w = q*100) facing quaternion (CCDDHEBKMGD)
//	u16 MovePlatformID (IABJNDBNDBK)  0 = world (widened from the request's 1 byte)
//	u32 RepID          (HLKLDEKEKOJ)  replication id -> the wall's own PRI (118/900)
//	bool HealthExtra   (FFEIEJNHHHJ)  extra-HP-pool flag (false)
//	bool Flag          (PDMCELIELNH)  1.70.1 appended bool (false)
func (w *Writer) iceWallState(s IceWallState) {
	w.U32(s.ID)
	w.I32(s.WallType)
	w.I32(s.HP)
	w.vec3i(r1000(s.Pos.X), r1000(s.Pos.Y), r1000(s.Pos.Z))
	w.I16(r100(s.Rot.X))
	w.I16(r100(s.Rot.Y))
	w.I16(r100(s.Rot.Z))
	w.I16(r100(s.Rot.W))
	w.U16(s.MovePlatformID)
	w.U32(s.RepID)
	w.Bool(s.HealthExtra)
	w.Bool(s.Flag)
}

// IceWallState is one gloo wall's replicated spawn state (message::JJIODFAHEOG).
type IceWallState struct {
	ID             uint32 // server-assigned wall entity/game id
	WallType       int32  // building/skin variant (echo the place request's WallType)
	HP             int32  // durability; 200 for a fresh wall
	Pos            Vec3   // world position (metres)
	Rot            Quat   // facing quaternion
	MovePlatformID uint16 // 0 = anchored to the world
	RepID          uint32 // replication id bound to this wall's PRI
	HealthExtra    bool   // extra-HP-pool flag (false)
	Flag           bool   // appended 1.70.1 bool (false)
}

// AddIcewall builds the cmd 218 RUDP_ADD_ICEWALL server->client SPAWN payload
// (message::KACGKFMOHNP): the 40-byte inner JJIODFAHEOG state followed by one
// trailing byte (LIBFAHCCCFA), the wall's initial STATE enum (0 = normal). 41
// bytes total. Broadcast to every player (including the placer). Verified from
// KACGKFMOHNP::UnSerialize @0x372b73c (inner UDPMessagePool<JJIODFAHEOG> + ReadByte).
func AddIcewall(s IceWallState, state byte) []byte {
	w := &Writer{}
	w.iceWallState(s)
	w.U8(state)
	return w.B
}

// ResyncIcewalls builds the cmd 220 RUDP_RESYNC_ICEWALL server->client payload
// (message::EKKLNKLGFIM): an i16 count followed by that many JJIODFAHEOG states
// (NO per-element trailing byte — that byte only exists on cmd 218). Send this to
// a (re)joining player so every already-placed wall reappears. Verified from
// EKKLNKLGFIM::UnSerialize @0x36d5470 (ReadInt16 count + list of JJIODFAHEOG).
func ResyncIcewalls(walls []IceWallState) []byte {
	w := &Writer{}
	w.I16(int16(len(walls)))
	for _, s := range walls {
		w.iceWallState(s)
	}
	return w.B
}

// RemoveIcewall builds the cmd 221 RUDP_REMOVE_ICEWALL server->client payload
// (message::APPOHELKIIG): a single u32 wall id (POBGKMDJMDC). Sent when a wall
// breaks (HP<=0), times out (~60s), or is evicted by the FIFO cap. 4 bytes.
// Verified from APPOHELKIIG::UnSerialize @0x37d83bc.
func RemoveIcewall(id uint32) []byte {
	w := &Writer{}
	w.U32(id)
	return w.B
}

// PlaceIcewallReq is the parsed cmd 218 RUDP_ADD_ICEWALL client->server PLACE
// request (message::JEGADMHBFKB). This is the inbound trigger the server handles
// to then spawn a wall via AddIcewall. Fixed 29-byte body, little-endian:
//
//	off 0  : u32  Unique          (PIAAFCGBIAA) placing player's building-item unique;
//	                              also the key used to decrement the gloo inventory
//	off 4  : i32  WallType        (LGBODPJOONM) building variant to echo back at spawn
//	off 8  : i32  x  (m*1000)     position (message::DEACEIFBHJK, CCIKDFGDBAM)
//	off 12 : i32  y  (m*1000)
//	off 16 : i32  z  (m*1000)
//	off 20 : i16  rx (q*100)      rotation (message::GAKIHFPIOEF, CCDDHEBKMGD)
//	off 22 : i16  ry (q*100)
//	off 24 : i16  rz (q*100)
//	off 26 : i16  rw (q*100)
//	off 28 : u8   MovePlatformID  (IABJNDBNDBK) 1 byte here; widened to u16 at spawn
//
// The CLIENT decides the position/rotation (server is authoritative only over the
// wall id, RepID and HP). Verified from JEGADMHBFKB::UnSerialize @0x37216bc.
type PlaceIcewallReq struct {
	Unique         uint32
	WallType       int32
	Pos            Vec3 // metres
	Rot            Quat
	MovePlatformID byte
}

// ParsePlaceIcewallReq decodes a cmd 218 client->server place request (29 bytes).
func ParsePlaceIcewallReq(payload []byte) (PlaceIcewallReq, bool) {
	if len(payload) < 29 {
		return PlaceIcewallReq{}, false
	}
	return PlaceIcewallReq{
		Unique:   binary.LittleEndian.Uint32(payload[0:]),
		WallType: int32(binary.LittleEndian.Uint32(payload[4:])),
		Pos: Vec3{
			X: float64(int32(binary.LittleEndian.Uint32(payload[8:]))) / 1000,
			Y: float64(int32(binary.LittleEndian.Uint32(payload[12:]))) / 1000,
			Z: float64(int32(binary.LittleEndian.Uint32(payload[16:]))) / 1000,
		},
		Rot: Quat{
			X: float64(int16(binary.LittleEndian.Uint16(payload[20:]))) / 100,
			Y: float64(int16(binary.LittleEndian.Uint16(payload[22:]))) / 100,
			Z: float64(int16(binary.LittleEndian.Uint16(payload[24:]))) / 100,
			W: float64(int16(binary.LittleEndian.Uint16(payload[26:]))) / 100,
		},
		MovePlatformID: payload[28],
	}, true
}

// IcewallDamageReq is the parsed cmd 219 RUDP_ICEWALL_TAKE_DAMAGE client->server
// request (message::CHDLJFJCPFN). The full message is large and variable-length
// (it ends with a List<float> and a List<byte>), but the server only needs the two
// leading fixed fields. Little-endian:
//
//	off 0 : u32 WallID  (ALFINFGBOBE) the target wall's id (matches IceWallState.ID)
//	off 4 : u16 _sub    (ECDBFHHNPMI) unused by the server
//	off 6 : u32 Damage  (BJBPPEBIPFA) damage amount (only the low 16 bits are set by
//	                    the client's IceWall::TakeDamage @0x26adf6c)
//	off 18: u32 Weapon  (HCMIEJEBKAL) firing weapon DataID (optional; read if present)
//
// Verified from CHDLJFJCPFN::UnSerialize @0x37e7c50.
type IcewallDamageReq struct {
	WallID uint32
	Damage uint32
	Weapon uint32
}

// ParseIcewallDamageReq decodes the fixed leading fields of a cmd 219 request. It
// needs at least WallID+_sub+Damage (10 bytes); Weapon is filled if the packet
// reaches it.
func ParseIcewallDamageReq(payload []byte) (IcewallDamageReq, bool) {
	if len(payload) < 10 {
		return IcewallDamageReq{}, false
	}
	r := IcewallDamageReq{
		WallID: binary.LittleEndian.Uint32(payload[0:]),
		Damage: binary.LittleEndian.Uint32(payload[6:]),
	}
	if len(payload) >= 22 {
		r.Weapon = binary.LittleEndian.Uint32(payload[18:])
	}
	return r, true
}
