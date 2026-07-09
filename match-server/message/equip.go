package message

import "encoding/binary"

// Inventory slot swap (cmd 119 EQUIP request + cmd 121 EquipmentChanged broadcast). Opcodes + wire
// confirmed against the FF 1.70.1 binary (Equip req = JHMFPMMDOLP @119, EquipmentChanged = LJGFNPIMGMA
// @121); both match the reference server. See [[cs-full-loadout]].

// EquipReq is a parsed cmd 119 (client->server, message JHMFPMMDOLP): the client dragged an inventory
// item onto a loadout slot. Wire (LE): [u8 SlotID][u32 UniqueID].
type EquipReq struct {
	Slot   byte
	Unique uint32
}

// ParseEquipReq decodes a cmd 119 payload; ok=false if it lacks the slot byte + unique.
func ParseEquipReq(payload []byte) (EquipReq, bool) {
	if len(payload) < 5 {
		return EquipReq{}, false
	}
	return EquipReq{Slot: payload[0], Unique: binary.LittleEndian.Uint32(payload[1:])}, true
}

// EquipmentChanged builds the cmd 121 (server->client, message LJGFNPIMGMA) payload: a visual slot
// transition on a pawn — the item that LEFT the slot (oldUID) and the item now IN it (newUID) with its
// DataID. Clients use it to move the weapon between loadout slots: the owner's inventory UI and remotes'
// back-mounted weapon models. Wire (LE): [u32 PlayerID][u8 SlotID][u32 OldUID][u32 NewUID][u32 DataID]
// [u32 RuntimeValue].
func EquipmentChanged(playerID uint32, slot byte, oldUID, newUID, dataID, runtime uint32) []byte {
	w := &Writer{}
	w.U32(playerID)
	w.U8(slot)
	w.U32(oldUID)
	w.U32(newUID)
	w.U32(dataID)
	w.U32(runtime)
	return w.B
}

// ChangeInventoryOnHand builds the cmd 108 (server->client, message JLMEBGKMNIL) payload that re-asserts
// which item a pawn holds: [u32 PlayerID][u32 UniqueID]. Sent after a slot swap so the moved held weapon
// doesn't drop the client back to fists. (The client also SENDS cmd 108 upstream with a distinct receive
// class — this is the S2C re-assert form.)
func ChangeInventoryOnHand(playerID, unique uint32) []byte {
	w := &Writer{}
	w.U32(playerID)
	w.U32(unique)
	return w.B
}
