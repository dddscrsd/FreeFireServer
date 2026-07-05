package message

// Weapon-attachment packets. cmd 122 RUDP_EQUIP_ATTACHMENT and cmd 123
// RUDP_UNEQUIP_ATTACHMENT are client->server requests; cmd 124 RUDP_ATTACHMENT_CHANGED
// (message KBDODAHANGB) is the server->client apply. The server force-equips maxed
// attachments onto a bought weapon by sending cmd 124.
//
// cmd 124 wire (from KBDODAHANGB::UnSerialize @0x372cffc). The apply handler
// NPCNMJAGIKI::FAMKDILDKOF @0x21b1920 resolves BOTH the weapon (BJDEIHKKJIC) and the new
// attachment (JPBPPOEJDKC) by UNIQUE from the player's inventory dict, so both must already
// have been added via cmd 174 before this arrives; it then installs the attachment.
//   - AMMPCFCENGO (field 4) and INOPGGABCAN (field 6) are decoded but NEVER read by the
//     handler -> send 0.
//   - PLNOMIBNNGI (scope aim-state bool) MUST be false: a value disagreeing with a scope's
//     current aim state on the LOCAL player self-inflicts 10000 damage (instant death).
//   - NLHCAOBDAPC (bool) = skip client consume/refund. Sent FALSE so the client consumes
//     the pushed attachment out of the backpack; sending true leaves it duplicated in both
//     the backpack and the weapon.

// AttachmentChanged builds a cmd 124 (KBDODAHANGB) force-equip: mount attachment `data`
// (instance `unique`, already registered via cmd 174) onto weapon `weaponUnique` in
// EAttachmentType `slot` (0=scope,1=muzzle,2=stock,3=foregrip,4=magazine), for the local
// player entity `entity`.
func AttachmentChanged(entity uint32, slot byte, weaponUnique, data, unique uint32) []byte {
	w := &Writer{}
	w.U32(entity)       // IHAAMHPPLMG — player entity id (routes to the local PlayerNetwork)
	w.U8(slot)          // EBOKCJFLINE — EAttachmentType slot on the weapon
	w.U32(weaponUnique) // BJDEIHKKJIC — target weapon instance unique
	w.U32(0)            // AMMPCFCENGO — ignored by the apply handler
	w.U32(data)         // ABFEHAPCGFE — new attachment DataID
	w.U32(0)            // INOPGGABCAN — ignored by the apply handler
	w.U32(unique)       // JPBPPOEJDKC — new attachment instance unique
	w.U8(0)             // PLNOMIBNNGI — scope aim-state; MUST be false (mismatch self-kills)
	w.U8(0)             // NLHCAOBDAPC — false: client consumes it from the backpack (true duplicated it)
	return w.B
}
