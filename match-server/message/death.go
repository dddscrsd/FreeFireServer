package message

// PlayerDead builds the cmd 107 (RUDP_DEAD, message OGCHKCGKGKN) payload: the dead
// entity, its killer, the killer's weapon (kill feed), the death position, and the
// ObserveID — the entity the dead client SPECTATES afterward. Reference udp.py
// (build_dead_payload_v143 field 6 "ObserveID") sets it to the dead player's living
// teammate, else the killer, else the nearest alive player; 0 = no spectate ("dry
// death"), which makes a solo dead player leave the match. The remaining kill-detail
// fields are left at their zero default. Field order verified from
// OGCHKCGKGKN::UnSerialize @0x346d518; the 1.70.1 layout appends 6 trailing fields over
// the reference's v1.43 death payload, all sent as 0 here.
func PlayerDead(victim, killer, weapon, weaponSkin, observe uint32, bodyPart uint8, pos Vec3, pendingRevive bool) []byte {
	w := &Writer{}
	w.U32(victim)                                     // IHAAMHPPLMG  dead entity
	w.U32(killer)                                     // MINLJOJIILD  killer
	w.I32(int32(weapon))                              // PIAMIOFEBKF  weapon data id (kill feed)
	w.U8(bodyPart)                                    // ODCJPCEJHPK  hit body part
	w.vec3i(r1000(pos.X), r1000(pos.Y), r1000(pos.Z)) // GPNLMNKMJII  death position (DEACEIFBHJK)
	w.U32(observe)                                    // CLOBKOJFKFK  ObserveID (spectate target)
	w.U16(0)                                          // IFIDKJLBGBH  container id
	w.Bool(false)                                     // BPHMFOOLLLP  killer bounty buff
	w.U32(weaponSkin)                                 // GAPBDHBOCBE  skin id
	w.Bool(pendingRevive)                             // EODDKBKMIHI  pending revive (keep the pawn alive-but-down so cmd 388 can revive it)
	w.Bool(false)                                     // KMOPGGEBJEF  dead by system
	w.U8(0)                                           // JPIGJLIALFD  kill stack
	w.U8(0)                                           // FHKHLGNHJPE  special kill
	w.U8(0)                                           // OIGAPMPJIBD  killer score
	w.U8(0)                                           // PMJGBAMEIBD  killer team score
	w.U8(0)                                           // APGNACACCIN  (1.70.1)
	w.U8(0)                                           // OKNJMHMBDNB  (1.70.1)
	w.U32(0)                                          // IAGFEBNBHLI  (1.70.1)
	w.U32(0)                                          // NPLIINOLPLI  (1.70.1)
	w.U32(0)                                          // JMKNNLFHFJC  (1.70.1)
	w.Bool(false)                                     // LCGMJMHJLNA  (1.70.1)
	return w.B
}
