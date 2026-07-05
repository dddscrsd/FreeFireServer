package main

import (
	"fmt"
	"log"
	"time"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// Ground-loot CONTAINER system. When an item is forced out of a player's loadout — buying
// a 3rd primary / 2nd pistol overrides one, or the client explicitly drops one — the
// overridden item spawns on the ground inside a CONTAINER anyone can walk to and pick up
// (cmd 227 AddContainer + cmd 114 AddPickup). A pickup (cmd 111) installs the ground item
// into the loadout (type-aware slot) and removes it from its container (cmd 115 DelPickup;
// cmd 228 DelContainer when the box empties). A CS match has ONE real client + a
// server-driven bot, so "everyone" == this session: every loot packet is a plain reliable
// send to s.remote (no broadcast loop). The bot never picks anything up.

// lootItem is one item lying on the ground inside a container. It KEEPS the item's original
// inventory Unique so the client can re-pick it with the same UID it already knows (the
// client sends that Unique in cmd 111); data/count/runtime mirror the InvItem it came from
// (runtime = a weapon's loaded magazine).
type lootItem struct {
	unique  uint32 // == the inventory InvItem.Unique it was dropped from (NOT re-allocated)
	data    uint32 // item DataID
	count   uint32
	runtime uint32 // loaded clip for a weapon, else 0
}

// container is one ground pickup box. id is the wire ContainerObjectID (uint16 namespace,
// disjoint from item Uniques and entity ids). pos is the box's world position in METRES
// (message.Vec3, same convention as s.playerPos / SpawnPos; serialized to int32 mm via
// r1000 by the wire builder). ctype: message.ContainerDynamic for player-dropped loot.
type container struct {
	id    uint16
	pos   message.Vec3
	ctype byte
	items []lootItem
}

const (
	// Runtime container-id band: disjoint from any static map-loot band and always inside
	// uint16 so DelContainer can't overflow. This CS server has no static map loot, so the
	// whole band is ours.
	runtimeContainerBase uint16 = 40000
	runtimeContainerMax  uint16 = 65535

	// mergeDistM is the proximity-merge radius in METRES (1.5 m == 1500 mm on the wire,
	// within the requested 1–2 m). Compared as squared distance to skip the sqrt.
	mergeDistM float64 = 1.5
)

// overrideDropDelay holds a purchase-displaced weapon in the bag this long before dropping it, so
// the client finishes equipping the newly-bought weapon (filling the slot) first. The client's
// weapon auto-pickup only fires into an EMPTY slot of the weapon's type (RE:
// NPCNMJAGIKI::IHOEHGHNBDN gates on HKAAOCIIMAD(slot)==null), so a FULL slot at drop time leaves the
// dropped weapon on the ground instead of yanking it back into the slot the buy just filled. Tunable.
const overrideDropDelay = 1 * time.Millisecond

// dropOverriddenAfterEquip removes (cmd 327) then ground-drops each weapon a purchase displaced,
// after overrideDropDelay so the client's auto-pickup sees the target slot already full. Runs in its
// own goroutine so the purchase handler doesn't block.
func (s *session) dropOverriddenAfterEquip(outgoing []lootItem) {
	time.Sleep(overrideDropDelay)
	pos := s.snapPlayerPos()
	var out []outPkt
	s.invMu.Lock()
	for _, it := range outgoing {
		out = s.dropToGroundLocked(it, pos, out)
	}
	s.invMu.Unlock()
	s.flush(out)
}

// outPkt is one queued outbound packet. Handlers BUILD their packets into a slice under
// invMu, release the lock, then flush — never send while holding a lock (sendDataLog takes
// s.mu for the seq counter).
type outPkt struct {
	cmd     uint16
	payload []byte
	label   string
}

// flush sends every queued packet in order. Call ONLY after invMu is released.
func (s *session) flush(out []outPkt) {
	for _, o := range out {
		s.sendDataLog(o.cmd, o.payload, o.label)
	}
}

// snapPlayerPos reads the local player's live world position under hpMu and releases it, so
// the caller can then take invMu (hpMu and invMu are never nested).
func (s *session) snapPlayerPos() message.Vec3 {
	s.hpMu.Lock()
	pos := s.playerPos
	s.hpMu.Unlock()
	return pos
}

// ensureLootLocked lazily initialises the container map + id allocator (the session is
// created bare). Caller holds invMu.
func (s *session) ensureLootLocked() {
	if s.containers == nil {
		s.containers = map[uint16]*container{}
	}
	if s.nextContainerID < runtimeContainerBase || s.nextContainerID > runtimeContainerMax {
		s.nextContainerID = runtimeContainerBase
	}
}

// allocContainerIDLocked returns the next runtime ContainerObjectID, monotonic with wrap
// INSIDE the band. Caller holds invMu.
func (s *session) allocContainerIDLocked() uint16 {
	s.ensureLootLocked()
	id := s.nextContainerID
	next := id + 1
	if next > runtimeContainerMax || next < runtimeContainerBase { // wrap inside the band
		next = runtimeContainerBase
	}
	s.nextContainerID = next
	return id
}

// pickupItem converts a lootItem to the wire PPickupInventory carried by cmd 114/115/225,
// tagged with its owning container id.
func (it lootItem) pickupItem(containerID uint16) message.PickupItem {
	return message.PickupItem{
		Unique:      it.unique,
		Data:        it.data,
		Count:       it.count,
		ContainerID: containerID,
		Runtime:     uint16(it.runtime),
	}
}

// weaponClass classifies an item DataID: (isWeapon, isPrimary). ammo 202 == pistol/secondary;
// any other weapon == primary. Mirrors placeItem's split so purchase and loot agree.
func weaponClass(data uint32) (isWeapon, isPrimary bool) {
	ammo, ok := weaponAmmo[data]
	if !ok {
		return false, false
	}
	return true, ammo != 202
}

// trackItemLocked records an item the client now holds. clientUIDs is the source of truth for
// the cmd 327 respawn clear AND for dropping ANY item (cmd 112) by unique — so consumables and
// throwables, which occupy no weapon slot, are still droppable. Caller holds invMu.
func (s *session) trackItemLocked(it lootItem) {
	if s.clientUIDs == nil {
		s.clientUIDs = map[uint32]lootItem{}
	}
	s.clientUIDs[it.unique] = it
}

// removeTrackedItemLocked fully removes the item with the given unique from the loadout: drops
// it from s.weapons + blanks its slot if it is a weapon, else blanks its equipment slot if it
// occupies one, untracks it, clears itemOnHand if held, and queues a cmd 327 so the client drops
// it too. Used by the manual drop (cmd 112) for ANY item type. Caller holds invMu.
func (s *session) removeTrackedItemLocked(unique uint32, out []outPkt) []outPkt {
	if slot, _, ok := s.findLoadoutWeaponLocked(unique); ok {
		delete(s.weapons, slot)
		s.clearEquipLocked(slot)
	} else {
		for _, e := range s.equipment {
			if e.Unique == unique {
				s.clearEquipLocked(e.Slot)
				break
			}
		}
	}
	delete(s.clientUIDs, unique)
	if s.itemOnHand == unique {
		s.itemOnHand = 0
	}
	out = append(out, outPkt{packet.CmdRemoveInventoryList,
		message.RemoveInventoryList(s.player.EntityID, []uint32{unique}),
		fmt.Sprintf("cmd=327 remove dropped uid=%d", unique)})
	return out
}

// dropToGroundLocked places `it` on the ground at pos (metres) with the PROXIMITY MERGE: if
// an existing container is within mergeDistM, the item is appended to it (the client already
// knows that box, so only a cmd 114 AddPickup carrying the box id+pos is emitted); otherwise
// a fresh container id is allocated and cmd 227 AddContainer + cmd 114 AddPickup are emitted.
// The fist (data==0) is never dropped. Returns the queued packets; caller holds invMu.
func (s *session) dropToGroundLocked(it lootItem, pos message.Vec3, out []outPkt) []outPkt {
	if it.unique == 0 && it.data == 0 { // fist is fixed, never dropped
		return out
	}
	s.ensureLootLocked()

	// Find the nearest existing container within the merge radius (squared distance, no sqrt).
	var box *container
	best := mergeDistM * mergeDistM
	for _, c := range s.containers {
		dx := c.pos.X - pos.X
		dy := c.pos.Y - pos.Y
		dz := c.pos.Z - pos.Z
		if d := dx*dx + dy*dy + dz*dz; d <= best {
			best, box = d, c
		}
	}

	if box == nil {
		// No neighbour → new container at pos. cmd 114 AddPickup ALONE materialises the
		// runtime box: the client's handler (FMONCEGKMFJ @0x1942054) routes it through
		// LevelObjectManager::IHAMOBOAELA keyed by the item's ContainerID + ContainerType, at
		// the item's position. cmd 227 AddContainer is a SEPARATE op that toggles PRE-PLACED
		// map level-objects (CKEDDPABIMN → GetLevelObjectType on a real level id), so sending
		// it for a runtime id is wrong — it must NOT be emitted here.
		id := s.allocContainerIDLocked()
		box = &container{id: id, pos: pos, ctype: message.ContainerDynamic}
		box.items = append(box.items, it)
		s.containers[id] = box
		out = append(out,
			outPkt{packet.CmdAddPickup,
				message.AddPickup(it.pickupItem(id), box.pos, box.ctype),
				fmt.Sprintf("cmd=114 AddPickup uid=%d data=%d -> box %d (new)", it.unique, it.data, id)})
		return out
	}
	// Merge into the existing box (snap the item to the box position).
	box.items = append(box.items, it)
	out = append(out,
		outPkt{packet.CmdAddPickup,
			message.AddPickup(it.pickupItem(box.id), box.pos, box.ctype),
			fmt.Sprintf("cmd=114 AddPickup uid=%d data=%d -> box %d (merge)", it.unique, it.data, box.id)})
	return out
}

// findLootLocked locates a ground loot item (and its container) by inventory unique. Caller
// holds invMu.
func (s *session) findLootLocked(unique uint32) (*container, lootItem, bool) {
	for _, c := range s.containers {
		for _, it := range c.items {
			if it.unique == unique {
				return c, it, true
			}
		}
	}
	return nil, lootItem{}, false
}

// removeItemFromContainerLocked slices the item with the given unique out of box. Caller
// holds invMu.
func removeItemFromContainerLocked(box *container, unique uint32) {
	out := box.items[:0]
	for _, it := range box.items {
		if it.unique != unique {
			out = append(out, it)
		}
	}
	box.items = out
}

// findLoadoutWeaponLocked returns the loadout slot + csWeapon holding the given unique.
// Caller holds invMu.
func (s *session) findLoadoutWeaponLocked(unique uint32) (byte, csWeapon, bool) {
	for slot, w := range s.weapons {
		if w.unique == unique {
			return slot, w, true
		}
	}
	return 0, csWeapon{}, false
}

// clearEquipLocked removes a slot's equipment entry (a removed weapon's slot goes empty).
// Caller holds invMu.
func (s *session) clearEquipLocked(slot byte) {
	out := s.equipment[:0]
	for _, e := range s.equipment {
		if e.Slot != slot {
			out = append(out, e)
		}
	}
	s.equipment = out
}

// removeLoadoutWeaponLocked fully removes the weapon in `slot` from the loadout: drops it
// from s.weapons, blanks its equipment slot, removes its unique from clientUIDs, and clears
// itemOnHand if it was held. It appends a cmd 327 RemoveInventoryList for the weapon's unique
// (the client must not hold that Unique while it also sits on the ground) and returns the
// updated queue + the removed weapon. The reserve ammo item is left intact. Caller holds invMu.
func (s *session) removeLoadoutWeaponLocked(slot byte, out []outPkt) ([]outPkt, csWeapon, bool) {
	w, ok := s.weapons[slot]
	if !ok {
		return out, csWeapon{}, false
	}
	delete(s.weapons, slot)
	s.clearEquipLocked(slot)
	delete(s.clientUIDs, w.unique)
	if s.itemOnHand == w.unique {
		s.itemOnHand = 0
	}
	out = append(out, outPkt{packet.CmdRemoveInventoryList,
		message.RemoveInventoryList(s.player.EntityID, []uint32{w.unique}),
		fmt.Sprintf("cmd=327 remove weapon uid=%d slot=%d (dropped)", w.unique, slot)})
	return out, w, true
}

// weaponLoot builds the lootItem for a weapon being dropped to the ground (full stack,
// full magazine).
func (s *session) weaponLoot(w csWeapon) lootItem {
	return lootItem{
		unique:  w.unique,
		data:    w.data,
		count:   1,
		runtime: loadedMagFor(w.data, SkinForWeapon(w.data, s.player.Slots)),
	}
}

// pickupWeaponSlotLocked resolves the TYPE-AWARE target slot for a picked-up weapon: a
// secondary/pistol always lands in SlotSecondary; a primary uses the SAME rule as a purchase —
// fill an EMPTY SlotPrimary1/SlotPrimary2 first, overriding the held primary (or P1) only when
// both are full. Filling an empty slot first is what lets a player who dropped BOTH primaries
// pick both back up (gun1->P1, gun2->P2); the old "override the held slot" rule kept sending the
// second gun back into P1, so the two swapped in P1 forever under auto-pickup. Caller holds invMu.
func (s *session) pickupWeaponSlotLocked(isPrimary bool) byte {
	if !isPrimary {
		return message.SlotSecondary
	}
	return s.pickPrimarySlotLocked()
}

// handlePickup handles cmd 111 (RUDP_PICKUP_INVENTORY): the client picks a ground item by its
// UniqueID. The item is installed into the loadout (type-aware slot; a weapon it displaces is
// itself dropped to the ground), added to the client inventory (cmd 174), then removed from
// its container (cmd 115), and the container is deleted (cmd 228) if it becomes empty.
func (s *session) handlePickup(p *packet.Packet) {
	req, ok := message.ParsePickupReq(p.Payload)
	if !ok {
		return
	}
	pos := s.snapPlayerPos()
	var pickEquips []attachEquip // maxed-attachment equips for a picked-up weapon (sent after flush)

	s.invMu.Lock()
	box, it, ok := s.findLootLocked(req.UniqueID)
	if !ok {
		s.invMu.Unlock()
		log.Printf("[mm-udp] cmd=111 pickup uid=%d not on ground — ignoring", req.UniqueID)
		return
	}
	var out []outPkt

	// The picked item leaves its container (in memory). Ground-removal packets are emitted
	// after the inventory sync so the item only disappears once the loadout has it.
	removeItemFromContainerLocked(box, it.unique)

	// Type-aware placement (weapons only). A weapon already in the target slot is dropped.
	if isWeapon, isPrimary := weaponClass(it.data); isWeapon {
		slot := s.pickupWeaponSlotLocked(isPrimary)
		var dropped *lootItem
		if old, occupied := s.weapons[slot]; occupied && old.unique != it.unique {
			var w csWeapon
			out, w, _ = s.removeLoadoutWeaponLocked(slot, out)
			ld := s.weaponLoot(w)
			dropped = &ld
		}
		// Install the picked weapon, REUSING its loot unique (no fresh nextUIDLocked) so the
		// client keeps the UID it already knows.
		s.weapons[slot] = csWeapon{slot: slot, data: it.data, unique: it.unique, ammo: weaponAmmo[it.data]}
		s.setEquipLocked(slot, it.data, it.unique)
		s.trackItemLocked(it)
		s.itemOnHand = it.unique

		// Max the picked weapon's attachments (same as a purchase) so a ground gun isn't stuck
		// at the default no-attachment state; the attachment items ride the cmd 174, cmd 124 mounts.
		var pickAttach []message.InvItem
		pickAttach, pickEquips = s.buildWeaponAttachmentsLocked(it.unique, it.data)

		// Add the picked item to the client inventory (cmd 174) BEFORE it leaves the ground.
		inv := []message.InvItem{{Unique: it.unique, Data: it.data, Count: it.count, Runtime: it.runtime}}
		equip := append([]message.Equipment(nil), s.equipment...)
		out = append(out, outPkt{packet.CmdSyncInventory,
			message.SyncInventory(s.player.EntityID, inv, pickAttach, equip, s.itemOnHand),
			fmt.Sprintf("cmd=174 SyncInventory (pickup data=%d uid=%d slot=%d, +%d attach)", it.data, it.unique, slot, len(pickAttach))})

		// Now spawn the displaced weapon on the ground (may merge into the box we just took
		// from, which then survives). Done after the 174 so its 327 already freed the unique.
		if dropped != nil {
			out = s.dropToGroundLocked(*dropped, pos, out)
		}
	} else {
		// Non-weapon (consumable/ammo/gloo): additive stack, no slot displacement.
		inv := []message.InvItem{{Unique: it.unique, Data: it.data, Count: it.count, Runtime: it.runtime}}
		s.trackItemLocked(it)
		equip := append([]message.Equipment(nil), s.equipment...)
		out = append(out, outPkt{packet.CmdSyncInventory,
			message.SyncInventory(s.player.EntityID, inv, nil, equip, s.itemOnHand),
			fmt.Sprintf("cmd=174 SyncInventory (pickup item data=%d uid=%d)", it.data, it.unique)})
	}

	// Remove the picked item from the ground for the client.
	out = append(out, outPkt{packet.CmdDelPickup,
		message.DelPickup(it.pickupItem(box.id), box.ctype),
		fmt.Sprintf("cmd=115 DelPickup uid=%d box=%d", it.unique, box.id)})

	// Delete the container if it is now empty (a displaced weapon may have re-merged into it,
	// in which case it survives).
	if len(box.items) == 0 {
		delete(s.containers, box.id)
		out = append(out, outPkt{packet.CmdDelContainer,
			message.DelContainer(box.id, box.ctype),
			fmt.Sprintf("cmd=228 DelContainer box=%d (empty)", box.id)})
	}
	s.invMu.Unlock()

	s.flush(out)
	s.sendAttachEquips(pickEquips) // mount the picked weapon's maxed attachments (after the cmd 174 flush)
}

// handleDrop handles cmd 112 (RUDP_DROP_INVENTORY): the client drops ANY inventory item — a
// weapon, a throwable, a gloo wall, a consumable, or ammo. It is removed from the loadout (cmd
// 327) and spawned on the ground at the player's position via the same container/merge path as
// an override drop. Any TRACKED unique is droppable, which is why non-weapons now appear on the
// ground (the old code only found loadout weapons and silently dropped everything else).
func (s *session) handleDrop(p *packet.Packet) {
	req, ok := message.ParseDropReq(p.Payload)
	if !ok {
		return
	}
	if req.Unique == 0 { // never drop the fist
		return
	}
	pos := s.snapPlayerPos()

	s.invMu.Lock()
	it, tracked := s.clientUIDs[req.Unique]
	if !tracked {
		s.invMu.Unlock()
		log.Printf("[mm-udp] cmd=112 drop uid=%d not a tracked item — ignoring", req.Unique)
		return
	}
	// A weapon carries its full magazine as runtime; every other item drops its tracked stack.
	var out []outPkt
	out = s.removeTrackedItemLocked(req.Unique, out)
	out = s.dropToGroundLocked(it, pos, out)
	s.invMu.Unlock()

	s.flush(out)
}
