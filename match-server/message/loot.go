package message

// Ground-loot CONTAINER / PICKUP packets for the Contra Squad drop/pickup system,
// reverse-engineered from FF 1.70.1. When an item is forced out of a player's loadout
// (buying a 3rd primary / 2nd pistol, or an explicit drop) it spawns on the ground
// inside a container anyone can walk to and pick up. In a CS match "everyone" is just
// the single local player, so these are sent to the session's own player only.
//
// Message-class / offset map (all verified from the client Serialize/UnSerialize):
//   cmd 227 RUDP_ADD_CONTAINER   = message::GONMBPDMEBE  (UnSerialize @0x368b3f8)
//   cmd 228 RUDP_DEL_CONTAINER   = message::OPNFJLJGPFC  (UnSerialize @0x34739e4)
//   cmd 114 RUDP_ADD_PICKUP      = message::INMIMMDMPFM  (UnSerialize @0x371c358)
//   cmd 225 RUDP_ADD_PICKUPLIST  = message::ANICDCDDCLN  (UnSerialize @0x37d6474)
//   cmd 115 RUDP_DEL_PICKUP      = message::KBHENKFBDAJ  (UnSerialize @0x372d23c)
//   PPickupInventory             = message::HDKLAINPKEP  (UnSerialize @0x368fa44, 21B)
//   position sub-msg             = message::DEACEIFBHJK  (3x i32 = m*1000; @0x36c6e9c)
//   cmd 111 RUDP_PICKUP_INVENTORY= message::LHODJLEHDND  (client->server, 5x u32)
//   cmd 112 RUDP_DROP_INVENTORY  = message::KJBONEENCAL  (client->server, u32,u32,u8)
// 1.70.1 APPENDS fields the older reference lacked: PPickupInventory +1 trailing byte
// (20->21B), AddPickup's MovePlatformID widened byte->u16 plus a trailing u32,
// AddPickupList uses i16 list counts (ref i32) and has a trailing empty i16-count list,
// and DropInventory gained a trailing Reason byte.

import "encoding/binary"

// EContainerType. Player-dropped runtime loot uses ContainerDynamic.
const (
	ContainerNormal  byte = 0
	ContainerDynamic byte = 1 // runtime player-dropped loot
	ContainerDeadBox byte = 2
	ContainerAirDrop byte = 3
)

// Container is one entry of a RUDP_ADD_CONTAINER (cmd 227) list (message::FFICCMDPDOA).
// 1.70.1 wire: [u32 Key][f32 Value][bool Flag] = 9 bytes. This element carries NO
// position/rotation/scale/bounds — Key's low byte is used client-side as the level-object
// type (LevelContainerBase.GetLevelObjectType), and Key references an existing level
// object. World placement of dropped loot is done via AddPickup (114) / AddPickupList
// (225), not this message. Verified: FFICCMDPDOA::UnSerialize @0x36dabf8.
type Container struct {
	Key   uint32  // BKFMPACNBLJ — container/level-object id+type key
	Value float32 // JOKDJFPBKFL — scalar (0 default)
	Flag  bool    // FMFLJAADBHG
}

// PickupItem is one PPickupInventory (message::HDKLAINPKEP): a loot entry inside a
// container. Serializes to exactly 21 bytes; reused by cmd 114/115/225. 1.70.1 appends a
// 7th field (Flag) over the older 6-field/20-byte reference. Verified from
// HDKLAINPKEP::UnSerialize @0x368fa44.
type PickupItem struct {
	Unique      uint32 // DGLCOGJJFMI  runtime unique id (the re-pickup key)
	Data        uint32 // PIAAFCGBIAA  item DataID
	Count       uint32 // HFPGENNDGME  stack count
	ContainerID uint16 // KMDFKJNCDNI  owning ContainerObjectID
	Runtime     uint16 // KFCDGPFPBEO  RunTimeValue (0 default)
	BountyOwner uint32 // ADCFAEPFDND  BountyOwnerID (0 = none)
	Flag        byte   // KGAJJIFBBGD  new in 1.70.1 (unknown; send 0)
}

// pickupInventory writes one HDKLAINPKEP (21 bytes). Shared by cmd 114/115/225.
func (w *Writer) pickupInventory(it PickupItem) {
	w.U32(it.Unique)
	w.U32(it.Data)
	w.U32(it.Count)
	w.U16(it.ContainerID)
	w.U16(it.Runtime)
	w.U32(it.BountyOwner)
	w.U8(it.Flag)
}

// AddContainer builds the cmd 227 payload (message::GONMBPDMEBE): an i16 count followed
// by that many 9-byte FFICCMDPDOA entries. Verified: GONMBPDMEBE::UnSerialize @0x368b3f8
// (ReadInt16 count — i16, NOT the reference's int32).
func AddContainer(containers []Container) []byte {
	w := &Writer{}
	w.I16(int16(len(containers)))
	for _, c := range containers {
		w.U32(c.Key)
		w.F32(c.Value)
		w.Bool(c.Flag)
	}
	return w.B
}

// DelContainer builds the cmd 228 payload (message::OPNFJLJGPFC):
// [u16 ContainerObjectID][u8 ContainerType] = 3 bytes. Sent when a container becomes
// empty after its last item is picked up. Verified: OPNFJLJGPFC::UnSerialize @0x34739e4.
func DelContainer(containerID uint16, containerType byte) []byte {
	w := &Writer{}
	w.U16(containerID)
	w.U8(containerType)
	return w.B
}

// AddPickup builds the cmd 114 RUDP_ADD_PICKUP payload (message::INMIMMDMPFM): one loot
// item on the ground at pos (world meters) inside a container of containerType.
// item.ContainerID MUST equal the owning container's object id. Wire = PPickupInventory(21)
// + pos(12) + ContainerType(1) + InWater(1) + MovePlatformID(2) + u32(4) = 41 bytes.
// Verified from INMIMMDMPFM::UnSerialize @0x371c358.
func AddPickup(item PickupItem, pos Vec3, containerType byte) []byte {
	w := &Writer{}
	w.pickupInventory(item)                           // LPOCFIBEEEJ  HDKLAINPKEP (21B)
	w.vec3i(r1000(pos.X), r1000(pos.Y), r1000(pos.Z)) // CCIKDFGDBAM  position (m*1000)
	w.U8(containerType)                               // LFNKFFLPPFF  ContainerType
	w.Bool(false)                                     // MJHGNJLONGO  InWater
	w.U16(0)                                          // IABJNDBNDBK  MovePlatformID (0 = world)
	w.U32(0)                                          // JIPJFMNJIKM  1.70.1 trailing field
	return w.B
}

// AddPickupList builds the cmd 225 RUDP_ADD_PICKUPLIST payload (message::ANICDCDDCLN):
// a single shared container position + the list of ground-loot items that live in it.
// All items share pos and their ContainerID must equal the container's object id.
// Wire = ContainerType(1) + InWater(1) + pos(12) + i16 count(2) + 21*N + MovePlatformID(2)
// + trailing empty i16-count list(2) = 18 + 21*N bytes. Verified from
// ANICDCDDCLN::UnSerialize @0x37d6474.
func AddPickupList(items []PickupItem, pos Vec3, containerType byte) []byte {
	w := &Writer{}
	w.U8(containerType)                               // LFNKFFLPPFF  ContainerType
	w.Bool(false)                                     // MJHGNJLONGO  InWater
	w.vec3i(r1000(pos.X), r1000(pos.Y), r1000(pos.Z)) // CCIKDFGDBAM  position (shared)
	w.I16(int16(len(items)))                          // DKBCFFIJAJG  count (i16!)
	for _, it := range items {
		w.pickupInventory(it) // HDKLAINPKEP[]
	}
	w.U16(0) // IABJNDBNDBK  MovePlatformID (u16!)
	w.I16(0) // OMPAHHFINCB  trailing OGNOJHPADCL list (empty)
	return w.B
}

// DelPickup builds the cmd 115 RUDP_DEL_PICKUP payload (message::KBHENKFBDAJ): removes one
// ground item so it disappears for the client. The client matches on item.Unique +
// item.ContainerID, so pass the same values used at Add. Wire = PPickupInventory(21) +
// ContainerType(1) = 22 bytes. Verified from KBHENKFBDAJ::UnSerialize @0x372d23c.
func DelPickup(item PickupItem, containerType byte) []byte {
	w := &Writer{}
	w.pickupInventory(item) // LPOCFIBEEEJ  HDKLAINPKEP (21B)
	w.U8(containerType)     // LFNKFFLPPFF  ContainerType
	return w.B
}

// PickupReq is the parsed cmd 111 RUDP_PICKUP_INVENTORY (client->server) request
// (message::LHODJLEHDND). Wire = 5x uint32 LE. The server matches the ground loot item by
// UniqueID ALONE; the trailing DataID/Count/RunTimeValue are client-populated but
// authoritative-ignored, and there is no container id / position in the request.
type PickupReq struct {
	PlayerID     uint32 // offset 0  — picker's PlayerID
	UniqueID     uint32 // offset 4  — loot item unique id (THE identifier)
	DataID       uint32 // offset 8  — advisory, ignore for matching
	Count        uint32 // offset 12 — advisory
	RunTimeValue uint32 // offset 16 — advisory
}

// ParsePickupReq parses a cmd 111 payload. The 1.70.1 client always serializes the full
// 20 bytes, but we accept >=8 (PlayerID+UniqueID is all the server needs) and zero-fill any
// absent trailing fields, mirroring the reference server's len>=8 guard.
func ParsePickupReq(payload []byte) (PickupReq, bool) {
	if len(payload) < 8 {
		return PickupReq{}, false
	}
	r := PickupReq{
		PlayerID: binary.LittleEndian.Uint32(payload[0:]),
		UniqueID: binary.LittleEndian.Uint32(payload[4:]),
	}
	if len(payload) >= 12 {
		r.DataID = binary.LittleEndian.Uint32(payload[8:])
	}
	if len(payload) >= 16 {
		r.Count = binary.LittleEndian.Uint32(payload[12:])
	}
	if len(payload) >= 20 {
		r.RunTimeValue = binary.LittleEndian.Uint32(payload[16:])
	}
	return r, true
}

// DropReq is a parsed cmd 112 RUDP_DROP_INVENTORY request (message::KJBONEENCAL).
// Wire (little-endian, 9 bytes): u32 Unique, u32 Count, u8 Reason. No drop position is
// transmitted — drop at the player's tracked pos (s.playerPos).
type DropReq struct {
	Unique uint32 // DGLCOGJJFMI — inventory item UniqueID to drop
	Count  uint32 // HFPGENNDGME — units to drop (weapons/single items = whole 1-count stack)
	Reason byte   // MBBDNIBDPME — drop reason enum; 0 for a normal player-initiated drop
}

// ParseDropReq decodes a cmd 112 payload; ok=false if it's too short. The older 8-byte
// form has no Reason (zero-filled).
func ParseDropReq(payload []byte) (DropReq, bool) {
	if len(payload) < 8 {
		return DropReq{}, false
	}
	r := DropReq{
		Unique: binary.LittleEndian.Uint32(payload[0:]),
		Count:  binary.LittleEndian.Uint32(payload[4:]),
	}
	if len(payload) >= 9 {
		r.Reason = payload[8]
	}
	return r, true
}

// DropInventoryRes builds the cmd 112 RUDP_DROP_INVENTORY response (same message class
// KJBONEENCAL, server->client): [u32 Unique][u32 remainingCount][u8 Reason]. The client
// sets the item's stack to remainingCount and removes it when 0. remaining =
// itemCount - min(Count, itemCount); a full weapon drop sends remainingCount=0, Reason=0.
func DropInventoryRes(unique, remainingCount uint32, reason byte) []byte {
	w := &Writer{}
	w.U32(unique)
	w.U32(remainingCount)
	w.U8(reason)
	return w.B
}
