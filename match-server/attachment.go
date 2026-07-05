package main

import (
	"fmt"
	"log"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// Weapon-attachment auto-max. When a weapon is bought, the server force-equips the best
// attachment for every slot the weapon supports — a convenience so players don't have to
// loot/equip attachments in a CS match. Each attachment is added as an inventory item
// (cmd 174) then mounted with cmd 124 (RUDP_ATTACHMENT_CHANGED); the client's manual
// unequip (cmd 123) is ignored so the attachments stay locked on.
//
// Two DISTINCT slot numberings are in play (do not conflate):
//   - EAttachmentType (below): the cmd-124 slot the client indexes a weapon's attachment
//     array by. From the reference + the cmd-124 apply handler NPCNMJAGIKI::FAMKDILDKOF.
//   - WeaponTable attachment_slot (weapons.go): the CSV's OWN list of which attachment
//     TYPES a weapon supports, in a separate numbering (0=muzzle,1=magazine,2=foregrip,
//     3=stock; 4/5 are shotgun/marksman-specific). Scope support is the sight_equip column.
//
// The cmd-124 apply does NOT validate against the CSV, so the CSV is only used to CHOOSE
// what to equip (correctness), never to gate client acceptance.

// EAttachmentType — the cmd-124 slot (EBOKCJFLINE) each attachment kind occupies on a
// weapon, from the game's AttachmentTable `slot` column (the authoritative source; the
// reference server's mapping was wrong and mis-slotted every attachment).
const (
	attachMuzzle   byte = 0
	attachMagazine byte = 1
	attachForegrip byte = 2
	attachSight    byte = 3
	attachStock    byte = 4
)

// Maxed attachment DataIDs (the best GENERAL-purpose tier of each kind, per AttachmentTable).
const (
	attMuzzleLv3   = 503 // Muzzle3
	attMagazineLv3 = 513 // Magezine3 — general Lv3; NOT 515 (Magezine5 is weapon_type 7 ONLY -> client rejects it as incompatible on normal guns)
	attForegripLv3 = 523 // Foregrip3
	attStockLv3    = 543 // Stock3
	attScope2x     = 534 // Sight4 (2x scope)
)

// maxAttach is one maxed attachment: the cmd-124 slot it occupies + its item DataID.
type maxAttach struct {
	slot byte
	data uint32
}

// csvSlotToAttach maps a WeaponTable attachment_slot index (which IS the EAttachmentType)
// to the maxed attachment for that slot. The sight (slot 3) is NOT here — it is gated by
// the per-weapon sight_equip whitelist (a weapon can list slot 3 yet accept no scopes, and
// snipers accept scopes without listing it). Slot 5 is a non-standard marksman slot.
var csvSlotToAttach = map[int]maxAttach{
	0: {attachMuzzle, attMuzzleLv3},     // muzzle
	1: {attachMagazine, attMagazineLv3}, // magazine
	2: {attachForegrip, attForegripLv3}, // foregrip
	4: {attachStock, attStockLv3},       // stock
}

// weaponMaxAttachments returns the maxed attachments to force onto a just-bought weapon:
// one per supported attachment_slot (muzzle/magazine/foregrip/stock) plus a 2x scope when
// the weapon's sight_equip whitelist accepts one.
func weaponMaxAttachments(weaponID uint32) []maxAttach {
	var out []maxAttach
	for _, slot := range weaponAttachSlots[weaponID] {
		if a, ok := csvSlotToAttach[slot]; ok {
			out = append(out, a)
		}
	}
	for _, sight := range weaponSights[weaponID] {
		if sight == attScope2x {
			out = append(out, maxAttach{attachSight, attScope2x})
			break
		}
	}
	return out
}

// attachEquip is one pending cmd 124 attachment force-equip for a bought weapon.
type attachEquip struct {
	weaponUnique uint32
	slot         byte
	data         uint32
	unique       uint32
}

// buildWeaponAttachments issues a maxed attachment ITEM for each slot `weaponData`
// supports, tracking each unique, and returns the InvItems to register via cmd 174 plus
// the cmd 124 equip records.
func (s *session) buildWeaponAttachments(weaponUnique, weaponData uint32) ([]message.InvItem, []attachEquip) {
	maxes := weaponMaxAttachments(weaponData)
	if len(maxes) == 0 {
		return nil, nil
	}
	inv := make([]message.InvItem, 0, len(maxes))
	equips := make([]attachEquip, 0, len(maxes))
	for _, m := range maxes {
		uid := s.nextUID()
		inv = append(inv, message.InvItem{Unique: uid, Data: m.data, Count: 1})
		s.trackItem(lootItem{unique: uid, data: m.data, count: 1})
		equips = append(equips, attachEquip{weaponUnique: weaponUnique, slot: m.slot, data: m.data, unique: uid})
	}
	return inv, equips
}

// loadedMagFor returns a weapon's loaded-magazine size (the InvItem.Runtime) = the base clip.
//
// The auto-magazine CLIP boost is intentionally DROPPED. RE (KANLCBHFONB): the client caps the
// loaded clip to the weapon's max-clip field JIPFKPDKJHH (@0x494), which is copied from the item
// stats ONCE at weapon-init (APJHPGBALDJ) and is never re-raised by a runtime cmd-124 magazine
// mount — its setter LMIANLIEKGE and the init are vtable-only with no reachable caller, so the
// stat recalc is tied to the client's own UI equip and a server-pushed cmd 124 never triggers it.
// A boosted Runtime is just clamped straight back to base. The magazine still mounts (visual);
// only the loaded clip stays at base. Kept as a seam so the boost can return if the recompute
// trigger is ever found.
func loadedMagFor(weaponID, skinID uint32) uint32 {
	return ClipSize(weaponID, skinID)
}

// sendAttachEquips fires a cmd 124 force-equip for each maxed attachment. The weapon and its
// attachment items must already have been registered via a preceding cmd 174.
func (s *session) sendAttachEquips(equips []attachEquip) {
	for _, ae := range equips {
		s.sendDataLog(packet.CmdAttachmentChanged,
			message.AttachmentChanged(s.player.EntityID, ae.slot, ae.weaponUnique, ae.data, ae.unique),
			fmt.Sprintf("cmd=124 equip attach data=%d slot=%d -> weapon uid=%d", ae.data, ae.slot, ae.weaponUnique))
	}
}

// handleEquipAttachment logs a cmd 122 client equip request. The server auto-maxes
// attachments on buy (cmd 124), so a manual client equip needs no server action.
func (s *session) handleEquipAttachment(p *packet.Packet) {
	log.Printf("[mm-udp] cmd=122 EQUIP_ATTACHMENT (client) %x — ignored (server auto-maxes)", p.Payload)
}

// handleUnequipAttachment DROPS a cmd 123 client unequip request: attachments are locked
// on. The server sends no cmd 124 back, so the client's request goes unconfirmed and the
// attachment stays mounted.
func (s *session) handleUnequipAttachment(p *packet.Packet) {
	log.Printf("[mm-udp] cmd=123 UNEQUIP_ATTACHMENT (client) %x — IGNORED (attachments locked on)", p.Payload)
}
