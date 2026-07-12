package message

// message::GKBDLJFGGMI (cmd 101, PLAYER_JOIN) and its nested messages, built to
// match the client's UnSerialize field order exactly. Identity (account id,
// entity id, name) is always populated; the selected cosmetics carried in
// PlayerInfo are available to the serializers below (their exact wire slots in
// OGJKHJAFNHB/PKPAMKEDCDC are still being reverse-engineered).
//
// Handler COW::GamePlay::KPDMJKOEHEE::OFEBIEPAIKH reads:
//   DCAFPIJDJEL.MIJOCMKONAD (u64) -> userID
//   DCAFPIJDJEL.IHAAMHPPLMG (u32) -> entity id -> IHAAMHPPLMG::CFFPIACECIG
//   Player::IsLocalPlayer(userID, entity) decides "is this our player"
//   then NFJPHMKKEBF::LONDNBHBPDO(match, userID, DBBDCBDBDIG, entity, ...) adds it.

// PlayerInfo is everything cmd 101 PLAYER_JOIN needs about a joining player:
// identity plus the selected cosmetics ("show"). It lives here (not in package
// main) so the serializers can build from it — a main-package type can't be
// imported by other packages. main.resolvePlayer fills it from the prepare_token
// JWT (see match-server/token Claims/Show); main aliases it as `joinPlayer`.
type PlayerInfo struct {
	AccountID uint64
	EntityID  uint32
	Name      string
	Role      uint32
	// MatchID is the prepare_token `mid` — the shared match id used as the settlement
	// idempotency key. Carried here for convenience; NOT written to the cmd 101 wire.
	MatchID uint64

	// Selected cosmetics, sourced from the JWT `show` claim / account
	// selected_items. Wire these into the fields below as their meanings are
	// confirmed; use w.U32List for the list<u32> cosmetic slots.
	Avatar  uint32   // avatar_id (character)
	Color   uint32   // skin_color
	Head    uint32   // head_pic
	Banner  uint32   // banner_id
	Clothes []uint32 // clothes item ids
	Slots   []uint32 // loadout slot item ids
	Emotes  []uint32 // emote item ids
	// BattleFlag is the equipped battle-flag config id, wired into PKPAMKEDCDC.MDDDNKCOMDF. The client reads
	// it into Player.BattleFlagID; NONZERO is what lets it plant a flag emote (send cmd 0x114). The flag's
	// actual model resolves from the emote link_id, so any nonzero value enables all flag emotes. See [[cs-emotes]].
	BattleFlag uint32
	TeamIdx    uint8 // the player's index on the team (on-wire cmd-101 field CPDNBJNLAIM)
	// PreferTeam is the custom-room team the host arranged (1 or 2), from the prepare_token. NOT
	// serialized — it's a match-server placement hint consumed by allocSlot. 0 => server balance-fills.
	PreferTeam uint8

	// Shows are the player's showcase item ids (selected_items.shows). Written into the cmd-101
	// cosmetics; the client buckets them by CollectionSubType into BaseProfileInfo.Shows, whose
	// Shows[7] (weapon) is the DISPLAY weapon the result screen + lobby/profile render.
	Shows []uint32

	// Spawn transform, written into the OGJKHJAFNHB sub-object of cmd 101
	// (CCIKDFGDBAM position ×1000 / CCDDHEBKMGD facing ×100). This is the
	// AUTHORITATIVE spawn — the client places the local player here, so no
	// force-teleport is needed and there is no drift-back. Zero = world origin.
	SpawnPos  Vec3
	SpawnFace Vec3
}

// hjln writes message::HJLNLDIMIMK (3x int16).
func (w *Writer) hjln(a, b, c int16) { w.I16(a); w.I16(b); w.I16(c) }

// ogjkhjafnhb writes message::OGJKHJAFNHB (the player profile sub-message).
func (w *Writer) ogjkhjafnhb(pi PlayerInfo) {
	w.U64(pi.AccountID) // MIJOCMKONAD  (account / user id)
	w.U64(0)            // PAJPAJAJKAF
	w.U32(pi.EntityID)  // IHAAMHPPLMG  (entity / player id)
	w.Str(pi.Name)      // GCJBNAHBGMD  (player name)
	// Spawn transform: CCIKDFGDBAM (position, DEACEIFBHJK ×1000) + CCDDHEBKMGD
	// (facing, HJLNLDIMIMK ×100). The client spawns the local player here.
	w.vec3i(r1000(pi.SpawnPos.X), r1000(pi.SpawnPos.Y), r1000(pi.SpawnPos.Z)) // CCIKDFGDBAM : DEACEIFBHJK (3x i32)
	w.hjln(r100(pi.SpawnFace.X), r100(pi.SpawnFace.Y), r100(pi.SpawnFace.Z))  // CCDDHEBKMGD : HJLNLDIMIMK (3x i16)
	w.Bool(false)                                                             // CBAAEJPGJHB
	w.U8(0)                                                                   // GHGCGGOLKIP
	w.U8(0)                                                                   // KIFHHPBIOHK
	w.I16(0)                                                                  // BPGLMCIMGJG  : List<JOLBCNEHKLJ> (count=0)
	w.U8(pi.TeamIdx)                                                          // CPDNBJNLAIM
	w.U32(0)                                                                  // PJBNKKBLEBB
	w.U32(0)                                                                  // ELLLMKJCGCD
	w.U8(0)                                                                   // AAEJAPCDKFG
	w.U16(0)                                                                  // DOOOGCFLAOG
	w.U8(0)                                                                   // KIHEIBKPGOB
	w.U32(0)                                                                  // MGFPABPGMCA
	w.U8(0)                                                                   // JAKEIPJNLJK
	w.Bool(false)                                                             // EODDKBKMIHI
	w.U32(0)                                                                  // DJJCMAJHBOG
	w.U16(0)                                                                  // LJJJLIDKLDC
	w.U16(0)                                                                  // KCGJHECIDBA
	w.U32(0)                                                                  // HBIFDLKGJEI
	w.U32(0)                                                                  // JDBMDHFFAHL
	w.U8(0)                                                                   // GPOFBNCDLCD
	w.Str("")                                                                 // BKELBEFMJPF
	w.U64(0)                                                                  // DBBDCBDBDIG
	w.U32(pi.Role)                                                            // KFMJKFECHGI  (role)
	w.U16(0)                                                                  // LLHLOCCAJGA
	w.Bool(false)                                                             // JHKDNBCFKMN
}

// pkpamkedcdc writes message::PKPAMKEDCDC (the player cosmetics sub-message).
// `pi` carries the selected cosmetics (avatar, clothes, slots, ...) — wire them
// into the fields below as their meanings are confirmed (currently all default
// to zero/empty so the join stays known-good). Note the two "read-ahead" list
// fields (MCPGFPHMOGM, GLHCLEMMLDN): the client always reads one trailing u32
// after the count, so count=0 => i16(0)+u32(0). Use w.U32List for plain
// list<u32> fields (COPDEIABDCP, CLJLCLADHNP, IOPNKHPNDDH, OJENBEIOBGL).
func (w *Writer) pkpamkedcdc(pi PlayerInfo) {
	w.U32(pi.Avatar)      // NIDNCDNNGME
	w.U32(pi.Color)       // FJNBPPGHKEO
	w.U32List(pi.Clothes) // COPDEIABDCP : List<u32> (count=0)
	w.I16(0)              // MCPGFPHMOGM : List<u32> (count=0, read-ahead)
	w.U32(pi.Banner)      //   GFAAFGKJNCN (trailing u32 of MCPGFPHMOGM)
	w.U32(pi.Head)        // HNMAFLDDGOK
	w.Str("")             // IAGGMHCJINB
	w.U32(0)              // FIGHLIMBFAB
	w.U64(0)              // ALPLJCJJCFI
	w.U32(0)              // GKEJIDBNKKN
	w.U32(0)              // AMOLNHICINI
	w.U32(0)              // BNMIJIJLBLD
	w.U32List(pi.Slots)   // CLJLCLADHNP : List<u32> (count=0)
	w.Bool(false)         // HGOLHAJOKNI
	w.U32(0)              // PGBDNENMCGB (badge count)
	w.U32(0)              // CFOEDHFJKCJ (badge id)
	w.U32(0)              // NBDEAJJACBM
	w.Bool(false)         // AKGKHJLNBHK
	w.U32List(pi.Slots)   // IOPNKHPNDDH : List<u32> (count=0)
	w.Str("")             // MFLBOPEBOKE
	w.Str("")             // AIBGAEMOLAN
	w.U32(0)              // LGFOLJMCMFH
	// OJENBEIOBGL : showcase list -> the client buckets it by CollectionSubType into BaseProfileInfo.Shows;
	// Shows[7] (weapon) is the DISPLAY weapon on the result screen + lobby/profile. This is the LAST of the
	// three list<u32> cosmetics fields, positionally matching Shows (the last collection dict, @0xe0). If the
	// result weapon is still wrong in-client, move pi.Shows to CLJLCLADHNP or IOPNKHPNDDH (data-only, no crash).
	w.U32List(pi.Shows)   // OJENBEIOBGL : showcase items (was mis-fed pi.Slots — the character-skill array)
	w.I16(0)              // GLHCLEMMLDN : List<u32> (count=0)
	w.U32(pi.BattleFlag)  // MDDDNKCOMDF : equipped BattleFlagID (@+0xB0) — NONZERO gates the flag-emote plant (client sends cmd 0x114)
	w.U32(0)              // FHMMPDFIPMJ (pin id)
	w.U32(0)              // ALELGKLAAOK
	w.U32(0)              // PAPFCAJCNPO
	w.U32(0)              // LJPHJJLJHBA
	w.U8(0)               // DKBKPCANCAA (platform)
	w.U16(0)              // ECKPKEMKIIM
	w.U32(0)              // GJBAMJFMEFN
	w.U32(0)              // CPDNBJNLAIM
}

// PlayerJoin builds message::GKBDLJFGGMI (cmd 101) from pi. clockTick is the server's current match-tick
// (matchSeconds / 0.033); it goes in field CEDJCPLOLNE, which the client uses ONLY on the LOCAL player's
// cmd 101 to anchor its server clock: KOMMANDCOCL = localTick − CEDJCPLOLNE, so CurrentServerTime becomes
// 0.033 × CEDJCPLOLNE at join, then advances at 1/s. It must be the match-tick, NOT the EntityID — putting
// the EntityID (~16.7M) here inflated CurrentServerTime to ~553,000 s and broke EVERY seconds-based
// countdown (the waiting overlay AND the buy-phase timer). Identity comes from the uid + the cmd 900/901
// layer, so this field is purely the clock anchor. See [[cs-waiting-cancel-lock]].
func PlayerJoin(pi PlayerInfo, clockTick uint32) []byte {
	w := &Writer{}
	w.ogjkhjafnhb(pi) // DCAFPIJDJEL : OGJKHJAFNHB
	w.U32(clockTick)  // CEDJCPLOLNE — LOCAL player's server-clock anchor (match-tick), not the EntityID
	w.pkpamkedcdc(pi) // IFHIJHODIKE : PKPAMKEDCDC
	w.I32(0)          // NKINFDBFNGJ
	w.I32(0)          // GPIPIFLLOGN
	w.U8(0)           // FBCCMPIJBNC (platform)
	return w.B
}

func PlayerQuit(result uint8) []byte {
	w := &Writer{}
	w.U8(result) // CNBBKLIEPFL (result)
	return w.B
}
