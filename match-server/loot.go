package main

import (
	"fmt"
	"log"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// Ground-loot CONTAINER system. When an item is forced out of a player's loadout — buying
// a 3rd primary / 2nd pistol overrides one, or the client explicitly drops one — the
// overridden item spawns on the ground inside a CONTAINER anyone can walk to and pick up
// (cmd 227 AddContainer + cmd 114 AddPickup). A pickup (cmd 111) installs the ground item
// into the loadout (type-aware slot) and removes it from its container (cmd 115 DelPickup;
// cmd 228 DelContainer when the box empties). The ground items are shared world state, so a
// drop/pickup's world packets (AddPickup / DelPickup / container add/del) fan out to the
// WHOLE roster — every player sees a loose item appear and vanish — while the owner-only
// inventory syncs (cmd 174 / cmd 327) reach just the picker/dropper (flush routes by cmd).
// The container map lives on the Match, so any player can pick up any player's drop. The bot
// never picks anything up.

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

// dropOverriddenAfterEquip removes (cmd 327) then ground-drops each weapon a purchase displaced.
// It runs inline on the match loop AFTER the new weapon's equip (cmd 174) has been queued, so the
// drop is the last packet out and the client — which processes our reliable sends IN ORDER — has
// filled the slot before the drop lands. The client's weapon auto-pickup only fires into an EMPTY
// slot of the weapon's type (RE: NPCNMJAGIKI::IHOEHGHNBDN gates on HKAAOCIIMAD(slot)==null), so a
// FULL slot at drop time leaves the weapon on the ground instead of yanking it back into the slot
// the buy just filled. Was a 1ms-delayed goroutine; run()'s single-owner, in-order sends make the
// delay (and the goroutine, which would now race run()) unnecessary.
func (s *session) dropOverriddenAfterEquip(outgoing []lootItem) {
	pos := s.snapPlayerPos()
	var out []outPkt
	for _, it := range outgoing {
		out = s.dropToGround(it, pos, out)
	}
	s.flush(out)
}

// outPkt is one queued outbound packet. Handlers build their packets into a slice, then flush
// them in order — a tidy batch kept from when sends couldn't happen inside the old locked
// sections (run() owns the state now, so it is purely for readable ordering).
type outPkt struct {
	cmd        uint16
	payload    []byte
	label      string
	unreliable bool
}

// flush sends every queued packet in order.
func (m *Match) flushBroadcast(out []outPkt) {
	for _, o := range out {
		if o.unreliable {
			m.broadcastDataUnreliable(o.cmd, o.payload, o.label)
		} else {
			m.broadcastData(o.cmd, o.payload, o.label)
		}
	}
}

// worldLoot reports whether a queued loot packet is shared ground-world state (a loose item
// or its container) that EVERY player must see, as opposed to an owner-only inventory sync
// (cmd 174 SyncInventory / cmd 327 RemoveInventoryList) that only the picker/dropper needs.
func worldLoot(cmd uint16) bool {
	switch cmd {
	case packet.CmdAddPickup, packet.CmdDelPickup, packet.CmdAddPickupList,
		packet.CmdAddContainer, packet.CmdDelContainer:
		return true
	}
	return false
}

// flush sends every queued packet in order: shared ground-loot spawns/removals broadcast to
// the whole roster (so a dropped item appears + vanishes for everyone), while the owner-only
// inventory syncs go to this session alone. The held-weapon model that others render still
// rides the PRI (field 4), so a picked-up gun shows on the picker without broadcasting cmd 174.
func (s *session) flush(out []outPkt) {
	for _, o := range out {
		if worldLoot(o.cmd) {
			s.match.broadcastData(o.cmd, o.payload, o.label)
		} else {
			s.sendDataLog(o.cmd, o.payload, o.label)
		}
	}
}

// snapPlayerPos returns the local player's last-reported world position (cmd 1001).
func (s *session) snapPlayerPos() message.Vec3 {
	pos := s.playerPos
	return pos
}

// ensureLoot lazily initialises the container map + id allocator (the session is
// created bare).
func (s *session) ensureLoot() {
	if s.match.containers == nil {
		s.match.containers = map[uint16]*container{}
	}
	if s.match.nextContainerID < runtimeContainerBase || s.match.nextContainerID > runtimeContainerMax {
		s.match.nextContainerID = runtimeContainerBase
	}
}

// allocContainerID returns the next runtime ContainerObjectID, monotonic with wrap
// INSIDE the band.
func (s *session) allocContainerID() uint16 {
	s.ensureLoot()
	id := s.match.nextContainerID
	next := id + 1
	if next > runtimeContainerMax || next < runtimeContainerBase { // wrap inside the band
		next = runtimeContainerBase
	}
	s.match.nextContainerID = next
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

// trackItem records an item the client now holds. clientUIDs is the source of truth for
// the cmd 327 respawn clear AND for dropping ANY item (cmd 112) by unique — so consumables and
// throwables, which occupy no weapon slot, are still droppable.
func (s *session) trackItem(it lootItem) {
	if s.clientUIDs == nil {
		s.clientUIDs = map[uint32]lootItem{}
	}
	s.clientUIDs[it.unique] = it
}

// trackedByData returns the tracked stack (unique + item) for a DataID, ignoring the unique
// `skip` (pass 0 to ignore none). Stackable consumables (gloo/medkit/grenade/ammo) MUST live
// as a SINGLE stack per data: callers merge a repeat add into the existing stack instead of
// allocating a second data=X item. Duplicate same-data stacks desync count-based use — e.g.
// gloo's glooCount() would decrement one of the duplicates at random, hit 0 early, and clear
// SlotBuilding while the player still holds walls.
func (s *session) trackedByData(data, skip uint32) (uint32, lootItem, bool) {
	for u, it := range s.clientUIDs {
		if u != skip && it.data == data {
			return u, it, true
		}
	}
	return 0, lootItem{}, false
}

// removeTrackedItem fully removes the item with the given unique from the loadout: drops
// it from s.weapons + blanks its slot if it is a weapon, else blanks its equipment slot if it
// occupies one, untracks it, clears itemOnHand if held, and queues a cmd 327 so the client drops
// it too. Used by the manual drop (cmd 112) for ANY item type.
func (s *session) removeTrackedItem(unique uint32, out []outPkt) []outPkt {
	if slot, _, ok := s.findLoadoutWeapon(unique); ok {
		delete(s.weapons, slot)
		s.clearEquip(slot)
	} else {
		for _, e := range s.equipment {
			if e.Unique == unique {
				s.clearEquip(e.Slot)
				break
			}
		}
	}
	delete(s.clientUIDs, unique)
	s.pruneWeaponAttach(unique) // forget its mounts so a late joiner isn't sent stale cmd-124s
	if s.itemOnHand == unique {
		s.itemOnHand = 0
	}
	out = append(out, outPkt{packet.CmdRemoveInventoryList,
		message.RemoveInventoryList(s.player.EntityID, []uint32{unique}),
		fmt.Sprintf("cmd=327 remove dropped uid=%d", unique), true})
	return out
}

// dropToGround places `it` on the ground at pos (metres) with the PROXIMITY MERGE: if
// an existing container is within mergeDistM, the item is appended to it (the client already
// knows that box, so only a cmd 114 AddPickup carrying the box id+pos is emitted); otherwise
// a fresh container id is allocated and cmd 227 AddContainer + cmd 114 AddPickup are emitted.
// The fist (data==0) is never dropped. Returns the queued packets.
func (s *session) dropToGround(it lootItem, pos message.Vec3, out []outPkt) []outPkt {
	if it.unique == 0 && it.data == 0 { // fist is fixed, never dropped
		return out
	}
	s.ensureLoot()

	// Find the nearest existing container within the merge radius (squared distance, no sqrt).
	var box *container
	best := mergeDistM * mergeDistM
	for _, c := range s.match.containers {
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
		id := s.allocContainerID()
		box = &container{id: id, pos: pos, ctype: message.ContainerDynamic}
		box.items = append(box.items, it)
		s.match.containers[id] = box
		out = append(out,
			outPkt{packet.CmdAddPickup,
				message.AddPickup(it.pickupItem(id), box.pos, box.ctype),
				fmt.Sprintf("cmd=114 AddPickup uid=%d data=%d -> box %d (new)", it.unique, it.data, id), false})
		return out
	}
	// Merge into the existing box (snap the item to the box position).
	box.items = append(box.items, it)
	out = append(out,
		outPkt{packet.CmdAddPickup,
			message.AddPickup(it.pickupItem(box.id), box.pos, box.ctype),
			fmt.Sprintf("cmd=114 AddPickup uid=%d data=%d -> box %d (merge)", it.unique, it.data, box.id), false})
	return out
}

// findLoot locates a ground loot item (and its container) by inventory unique.
func (s *session) findLoot(unique uint32) (*container, lootItem, bool) {
	for _, c := range s.match.containers {
		for _, it := range c.items {
			if it.unique == unique {
				return c, it, true
			}
		}
	}
	return nil, lootItem{}, false
}

// removeItemFromContainer slices the item with the given unique out of box.
func removeItemFromContainer(box *container, unique uint32) {
	out := box.items[:0]
	for _, it := range box.items {
		if it.unique != unique {
			out = append(out, it)
		}
	}
	box.items = out
}

// findLoadoutWeapon returns the loadout slot + csWeapon holding the given unique.
func (s *session) findLoadoutWeapon(unique uint32) (byte, csWeapon, bool) {
	for slot, w := range s.weapons {
		if w.unique == unique {
			return slot, w, true
		}
	}
	return 0, csWeapon{}, false
}

// clearEquip removes a slot's equipment entry (a removed weapon's slot goes empty).
func (s *session) clearEquip(slot byte) {
	out := s.equipment[:0]
	for _, e := range s.equipment {
		if e.Slot != slot {
			out = append(out, e)
		}
	}
	s.equipment = out
}

// removeLoadoutWeapon fully removes the weapon in `slot` from the loadout: drops it
// from s.weapons, blanks its equipment slot, removes its unique from clientUIDs, and clears
// itemOnHand if it was held. It appends a cmd 327 RemoveInventoryList for the weapon's unique
// (the client must not hold that Unique while it also sits on the ground) and returns the
// updated queue + the removed weapon. The reserve ammo item is left intact.
func (s *session) removeLoadoutWeapon(slot byte, out []outPkt) ([]outPkt, csWeapon, bool) {
	w, ok := s.weapons[slot]
	if !ok {
		return out, csWeapon{}, false
	}
	delete(s.weapons, slot)
	s.clearEquip(slot)
	delete(s.clientUIDs, w.unique)
	s.pruneWeaponAttach(w.unique) // forget its mounts so a late joiner isn't sent stale cmd-124s
	if s.itemOnHand == w.unique {
		s.itemOnHand = 0
	}
	out = append(out, outPkt{packet.CmdRemoveInventoryList,
		message.RemoveInventoryList(s.player.EntityID, []uint32{w.unique}),
		fmt.Sprintf("cmd=327 remove weapon uid=%d slot=%d (dropped)", w.unique, slot), false})
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

// pickupWeaponSlot resolves the TYPE-AWARE target slot for a picked-up weapon: a
// secondary/pistol always lands in SlotSecondary; a primary uses the SAME rule as a purchase —
// fill an EMPTY SlotPrimary1/SlotPrimary2 first, overriding the held primary (or P1) only when
// both are full. Filling an empty slot first is what lets a player who dropped BOTH primaries
// pick both back up (gun1->P1, gun2->P2); the old "override the held slot" rule kept sending the
// second gun back into P1, so the two swapped in P1 forever under auto-pickup.
func (s *session) pickupWeaponSlot(isPrimary bool) byte {
	if !isPrimary {
		return message.SlotSecondary
	}
	return s.pickPrimarySlot()
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
	before := s.equipSnapshot()  // capture the slot map so we can tell remotes which slots changed
	var pickEquips []attachEquip // maxed-attachment equips for a picked-up weapon (sent after flush)

	box, it, ok := s.findLoot(req.UniqueID)
	if !ok {
		log.Printf("[mm-udp] cmd=111 pickup uid=%d not on ground — ignoring", req.UniqueID)
		return
	}
	var out []outPkt

	// The picked item leaves its container (in memory). Ground-removal packets are emitted
	// after the inventory sync so the item only disappears once the loadout has it.
	removeItemFromContainer(box, it.unique)

	// Type-aware placement (weapons only). A weapon already in the target slot is dropped.
	if isWeapon, isPrimary := weaponClass(it.data); isWeapon {
		slot := s.pickupWeaponSlot(isPrimary)
		var dropped *lootItem
		if old, occupied := s.weapons[slot]; occupied && old.unique != it.unique {
			var w csWeapon
			out, w, _ = s.removeLoadoutWeapon(slot, out)
			ld := s.weaponLoot(w)
			dropped = &ld
		}
		// Install the picked weapon, REUSING its loot unique (no fresh nextUID) so the
		// client keeps the UID it already knows.
		s.weapons[slot] = csWeapon{slot: slot, data: it.data, unique: it.unique, ammo: weaponAmmo[it.data]}
		s.setEquip(slot, it.data, it.unique)
		s.trackItem(it)
		s.itemOnHand = it.unique

		// Max the picked weapon's attachments (same as a purchase) so a ground gun isn't stuck
		// at the default no-attachment state; the attachment items ride the cmd 174, cmd 124 mounts.
		var pickAttach []message.InvItem
		pickAttach, pickEquips = s.buildWeaponAttachments(it.unique, it.data)

		// Add the picked item to the client inventory (cmd 174) BEFORE it leaves the ground.
		inv := []message.InvItem{{Unique: it.unique, Data: it.data, Count: it.count, Runtime: it.runtime}}
		equip := append([]message.Equipment(nil), s.equipment...)
		out = append(out, outPkt{packet.CmdSyncInventory,
			message.SyncInventory(s.player.EntityID, inv, pickAttach, equip, s.itemOnHand),
			fmt.Sprintf("cmd=174 SyncInventory (pickup data=%d uid=%d slot=%d, +%d attach)", it.data, it.unique, slot, len(pickAttach)), false})

		// Now spawn the displaced weapon on the ground (may merge into the box we just took
		// from, which then survives). Done after the 174 so its 327 already freed the unique.
		if dropped != nil {
			out = s.dropToGround(*dropped, pos, out)
		}
	} else {
		// Non-weapon (consumable/ammo/gloo). A STACKABLE consumable must stay a SINGLE stack:
		// if the player already holds this data under another unique, FOLD that stack into the
		// freshly-picked unique (which the client keeps, like the weapon path above) and drop
		// the old one. Otherwise gloo/medkits split into duplicate data=X stacks, which desyncs
		// count-based use — gloo's glooCount() would decrement one stack at random and clear
		// SlotBuilding while walls remain. This also naturally stacks picked-up ammo.
		if u, existing, dup := s.trackedByData(it.data, it.unique); dup {
			it.count += existing.count
			heldSlot, _, hadSlot := s.equipSlotOf(u) // the slot the old stack occupied (e.g. SlotBuilding)
			wasHeld := s.itemOnHand == u
			out = s.removeTrackedItem(u, out) // cmd 327 the old stack + free its slot/held state
			if hadSlot {
				s.setEquip(heldSlot, it.data, it.unique) // re-point that slot at the surviving stack
			}
			if wasHeld {
				s.itemOnHand = it.unique
			}
		}
		inv := []message.InvItem{{Unique: it.unique, Data: it.data, Count: it.count, Runtime: it.runtime}}
		s.trackItem(it)
		equip := append([]message.Equipment(nil), s.equipment...)
		out = append(out, outPkt{packet.CmdSyncInventory,
			message.SyncInventory(s.player.EntityID, inv, nil, equip, s.itemOnHand),
			fmt.Sprintf("cmd=174 SyncInventory (pickup item data=%d uid=%d)", it.data, it.unique), false})
	}

	// Remove the picked item from the ground for the client.
	out = append(out, outPkt{packet.CmdDelPickup,
		message.DelPickup(it.pickupItem(box.id), box.ctype),
		fmt.Sprintf("cmd=115 DelPickup uid=%d box=%d", it.unique, box.id), false})

	// Delete the container if it is now empty (a displaced weapon may have re-merged into it,
	// in which case it survives).
	if len(box.items) == 0 {
		delete(s.match.containers, box.id)
		out = append(out, outPkt{packet.CmdDelContainer,
			message.DelContainer(box.id, box.ctype),
			fmt.Sprintf("cmd=228 DelContainer box=%d (empty)", box.id), false})
	}

	s.flush(out)
	s.broadcastEquipDiff(before)   // tell remotes the changed slot(s) so the picked-up weapon back-mounts (not just the held gun)
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

	it, tracked := s.clientUIDs[req.Unique]
	if !tracked {
		log.Printf("[mm-udp] cmd=112 drop uid=%d not a tracked item — ignoring", req.Unique)
		return
	}
	// A weapon carries its full magazine as runtime; every other item drops its tracked stack.
	before := s.equipSnapshot() // capture the slot map so we can tell remotes which slot emptied
	var out []outPkt
	out = s.removeTrackedItem(req.Unique, out)
	out = s.dropToGround(it, pos, out)

	s.flush(out)
	s.broadcastEquipDiff(before) // tell remotes the dropped weapon left its slot (clears their back-mount view)
}
