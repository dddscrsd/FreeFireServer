package message

// Replication sync (cmd 900 PRI / cmd 901 GRI in FF 1.70.1 — the ref's 500/501
// are for an older build; live-confirmed 900/901 here), reverse-engineered from
// GCommon::ReplicationDataPool::SyncReplicationData @0x33f4f5c (verified live in
// IDA). The client deserializes a replication block as:
//
//	blockLen : u16 (LE)                 -- byte count of the tuples that follow
//	repeat while (pos - start) < blockLen:
//	  typeCode : u8                     -- selects value width (table below)
//	  fieldId  : u8                     -- SetData<T>(fieldId, value) key
//	  value    : per typeCode (fixed-width LE; ENABLE_FAST_PROTO=false)
//
// cmd 901 (GRI, game-wide state) = a single block, routed to MatchGame's one
// GRI pool (handler KPDMJKOEHEE::PNJHGADAGFN -> MatchGame::OnSyncReplicationData).
// cmd 900 (PRI, per-player state) = repeated [RepID u32][outerLen u16][block] per
// entity (handler KPDMJKOEHEE::DNCBMEDLPGP). Player RepID is a sequential id from
// 1000 (props/vehicles use other ranges); the client adopts the FIRST EntityType=1
// binding (from cmd 118 BindPRI) as its own local player.
//
// Only changed fields need be sent; SetData just stores the value and fires that
// field's DataChangedHandler. Sending a fieldId with the WRONG typeCode calls a
// different-typed SetData<T>, so the client's typed handler won't fire — the
// typeCode must match how the field was registered (see the CS map below).

// Replication value type codes (the switch in SyncReplicationData).
const (
	RepI8   byte = 0
	RepU8   byte = 1
	RepI16  byte = 2
	RepU16  byte = 3
	RepI32  byte = 4
	RepU32  byte = 5
	RepBool byte = 6
	RepF32  byte = 7
	RepU64  byte = 8
	RepI64  byte = 9
)

// RepEntry is one replicated field tuple.
type RepEntry struct {
	Type  byte   // one of Rep* above
	Field byte   // fieldId (the SetData key registered via AddData)
	Value uint64 // interpreted per Type
}

// ReplicationBlock builds a replication block (u16 blockLen + tuples). This is
// the cmd 501 (GRI) payload directly, and the per-entity block used inside cmd
// 500 (PRI).
func ReplicationBlock(entries []RepEntry) []byte {
	body := &Writer{}
	for _, e := range entries {
		body.U8(e.Type)
		body.U8(e.Field)
		switch e.Type {
		case RepI8, RepU8, RepBool:
			body.U8(byte(e.Value))
		case RepI16, RepU16:
			body.U16(uint16(e.Value))
		case RepI32, RepU32, RepF32:
			body.U32(uint32(e.Value))
		case RepU64, RepI64:
			body.U64(e.Value)
		}
	}
	w := &Writer{}
	w.U16(uint16(len(body.B))) // blockLen (bytes of tuple data only)
	w.B = append(w.B, body.B...)
	return w.B
}

// --- Contra Squad (game mode 15) GRI ---------------------------------------
// The CS game class COW::GamePlay::JBCMHIAGMHA registers a 7-field GRI pool
// (JBCMHIAGMHA::InitGRIData @0x26b3e90). Field ids + wire types are verified
// from the AddData calls; semantics from each field's DataChangedHandler.
const (
	CSFieldMaxRound     byte = 1 // u8  -> maxRound (roundsToWin = (max+1)/2)
	CSFieldCurrentRound byte = 2 // u8  -> current round index (0-based; getter +1)
	CSFieldPhase        byte = 3 // u32 -> (ECSMatchPhase<<16) | phaseParam  [MOVEMENT GATE]
	CSFieldMatchPoint   byte = 4 // u8  -> matchPoint (bool)
	CSFieldDrawState    byte = 5 // u32 -> (EMatchDrawState<<16) | stateTimer
	CSFieldAirdrop      byte = 6 // u32 -> airdrop/care-package sync id
)

// ECSMatchPhase (JBCMHIAGMHA GRI field 3, high 16 bits). Waiting keeps the intro
// mask up and the spawn fences active (player frozen); Prepare disables the mask
// and opens the shop; Fight drops the fences so the local player can move.
const (
	CSPhaseWaiting      uint16 = 0
	CSPhasePrepare      uint16 = 1
	CSPhaseFight        uint16 = 2
	CSPhasePost         uint16 = 3
	CSPhaseCutscene     uint16 = 4
	CSPhaseIntroduction uint16 = 5
)

// EMatchDrawState (JBCMHIAGMHA GRI field 5; hi16 = state, lo16 = countdown deadline second). The
// pre-match WAITING/CANCEL overlay — a separate field from the field-3 round phase. RE'd states (client
// enum EFDIOPGODAE, handler JIKLJNHJOAG @0x26b5a38):
//
//	0 NormalStart — no overlay (normal play).
//	1 HalfwayJoin — "Waiting for players" countdown (T_25_YP_CS_WAITING_CONNECT); lo16 = deadline sec.
//	2 Draw        — "Match cancelled / cut short" (T_25_YP_CS_CUT_SHORT); the client fixes a 10s countdown.
//	3 MatchEnd    — ReturnToLobby (fires RETURN_TO_LOBBY reason 9 -> kicks the client back to the lobby).
//	4 CancelDraw  — Resume/clear the overlay, gameplay proceeds.
const (
	CSDrawNormalStart uint16 = 0
	CSDrawHalfwayJoin uint16 = 1
	CSDrawDraw        uint16 = 2
	CSDrawMatchEnd    uint16 = 3
	CSDrawCancelDraw  uint16 = 4
)

// packPhase packs a phase/state enum (hi16) with a param/timer (lo16) into the
// u32 the CS field 3 / field 5 handlers expect.
func packPhase(enum, param uint16) uint64 {
	return uint64(enum)<<16 | uint64(param)
}

// CSGRIInit builds the CS GRI block: round config (max + current round, 0-based). Streamed continuously;
// bump currentRound to advance rounds. It deliberately does NOT touch field 4 (matchPoint) — CSGRIMatchPoint
// owns that — NOR field 5 (draw/match-state) — CSGRIMatchState owns that. Both are separate to avoid the
// toggle bug: setting a field here to one value and then to another in the same tick flips it every stream,
// re-firing the client's change-handler each tick.
func CSGRIInit(maxRound, currentRound uint8) []byte {
	return ReplicationBlock([]RepEntry{
		{RepU8, CSFieldMaxRound, uint64(maxRound)},
		{RepU8, CSFieldCurrentRound, uint64(currentRound)},
	})
}

// CSGRIPhase builds a GRI block that sets only the match phase (field 3). param
// is the phase's lo16 (buy/round end time for the client countdown; 0 = none).
func CSGRIPhase(phase, param uint16) []byte {
	return ReplicationBlock([]RepEntry{
		{RepU32, CSFieldPhase, packPhase(phase, param)},
	})
}

// CSGRIMatchState builds a GRI block that sets only the match-state field (field 5, EMatchDrawState) — the
// pre-match waiting/cancel overlay. state is a CSDraw* value; deadline is the lo16 match-clock second the
// countdown targets (0 = none / the client fixes the duration). Sent as its OWN field so it isn't toggled
// against CSGRIInit each stream (see the CSDraw* enum comment).
func CSGRIMatchState(state, deadline uint16) []byte {
	return ReplicationBlock([]RepEntry{
		{RepU32, CSFieldDrawState, packPhase(state, deadline)},
	})
}

func CSGRIMatchPoint(point uint8) []byte {
	return ReplicationBlock([]RepEntry{
		{RepU8, CSFieldMatchPoint, uint64(point)},
	})
}

// --- PRI (cmd 900) + BindPRI (cmd 118) -------------------------------------
// Format from the working reference (udp.py: send_bind_pri / build_pri_hp_block /
// build_sync_pri_multi_entity) + IDA (DNCBMEDLPGP @0x194e080). BindPRI is reliable
// (so=2, encrypted); the PRI VAR sync (cmd 900) is so=4 plaintext unreliable and
// streamed continuously so the player stays alive.

// RepIDForEntity maps a level-object EntityGameID to its replication id (reference
// scheme: RepID = 500 + (EntityGameID - 1001); e.g. oildrum gameID 1001 -> 500).
func RepIDForEntity(entityGameID uint32) uint32 { return 500 + entityGameID - 1001 }

// BindPRI entity types (message FJAMLLEPEKN EntityTag; the switch in the cmd 118
// handler KPDMJKOEHEE::JPBEOILPAAA @0x194cf70 maps each tag to a level-object type).
const (
	BindEntityPlayer  byte = 1
	BindEntityVehicle byte = 3
	BindEntityOilDrum byte = 5
	BindEntityIceWall byte = 13 // tag 0xD -> OFJHNKMJNGA_IceWall; binds a gloo wall's PRI
)

// BindEntry binds a RepID to a game entity (one cmd 118 entry).
type BindEntry struct {
	RepID        uint32
	EntityType   byte
	EntityGameID uint32
}

// BindPRI builds the cmd 118 payload. This is the TYPED message IJFFCCLHLHK
// (handler KPDMJKOEHEE::JPBEOILPAAA @0x194cf70), NOT a raw blob. Verified from
// 1.70.1 IJFFCCLHLHK::Serialize @0x369aa54 + FJAMLLEPEKN::Serialize @0x36dce8c:
//
//	count : i16 (LE)                                  -- NOT u32! (was the bug)
//	repeat count times (FJAMLLEPEKN):
//	  ReplicationID (HLKLDEKEKOJ) : u32
//	  EntityTag     (FDMALNPFPOE) : u8  (1=Player, 3=Vehicle, 5=OilDrum, ...)
//	  EntityGameID  (HMGDLNDJNIM) : u32
//
// The handler finds the entity by (EntityTag, EntityGameID), then calls
// OnReplicationBind(entity, ReplicationID) which creates the entity's PRIDataPool
// (so its m_PRIDataPool goes non-null and GetReplicationID == ReplicationID). The
// local player must be the FIRST EntityType=1 entry. Sent reliably (so=2), like
// the other typed messages (cmd 100/101/130).
func BindPRI(entries []BindEntry) []byte {
	w := &Writer{}
	w.U16(uint16(len(entries))) // count is i16 (2 bytes), not u32
	for _, e := range entries {
		w.U32(e.RepID)
		w.U8(e.EntityType)
		w.U32(e.EntityGameID)
	}
	return w.B
}

// PRIEntity pairs a RepID with its (blockLen-prefixed) replication block.
type PRIEntity struct {
	RepID uint32
	Block []byte
}

// SyncPRI builds the cmd 900 PRI payload: per entity [RepID u32][block], where
// block == ReplicationBlock == [blockLen u16][tuples]. The client reads exactly ONE
// u16 per entity: DNCBMEDLPGP @0x194e080 -> OnSyncReplicationData @0x33f9078 ->
// ReplicationDataPool::SyncReplicationData @0x33f4f5c does a single ReadFixUInt16
// (= blockLen) then loops [u8 type][u8 field][value]. Do NOT prepend a second (outer)
// u16 length: the extra u16 makes the pool read the inner blockLen's LOW BYTE as a
// typeCode, mis-typing field 0. Large blocks (player) survived by luck — the stray
// byte hit the parser's unsupported-type default case and re-aligned — but a small
// block like the 1-field ice wall (block 03 00 01 00 01) got its 0x03 read as U16, so
// SetData<u16> ran instead of SetData<byte> and the crack DataChangedHandler never
// fired. One u16 per entity is correct for both.
func SyncPRI(entities []PRIEntity) []byte {
	w := &Writer{}
	for _, e := range entities {
		w.U32(e.RepID)
		w.B = append(w.B, e.Block...) // block already carries its own [blockLen u16]
	}
	return w.B
}

// PRIHealHP builds a minimal cmd 900 PRI carrying ONLY the entity's new current HP (field
// 0 CUR_HP, u16). SetData is per-field on the client, so a lone field-0 update just moves
// the HP bar — used by the medkit heal-over-time to stream 5 HP steps without re-sending
// the full player block (which the regular ~3/s stream still carries).
func PRIHealHP(repID uint32, hp uint16) []byte {
	return SyncPRI([]PRIEntity{
		{RepID: repID, Block: ReplicationBlock([]RepEntry{{RepU16, 0, uint64(hp)}})},
	})
}

func PRIArmorDur(repID uint32, vest, helmet uint16) []byte {
	return SyncPRI([]PRIEntity{
		{RepID: repID, Block: ReplicationBlock([]RepEntry{{RepU64, 2, uint64(vest)}, {RepU64, 3, uint64(helmet)}})},
	})
}

func PRIRescuing(repID uint32, rescuing uint8) []byte {
	return SyncPRI([]PRIEntity{
		{RepID: repID, Block: ReplicationBlock([]RepEntry{{RepU8, 5, uint64(rescuing)}})},
	})
}

// PRIHPBlock builds a full alive-player PRI block. The field ids/types/order match
// the reference's build_pri_hp_block exactly (derived from the client's
// OnUserDefineReplicationInfo @0x190676c AddData registration). SetData is keyed by
// fieldId so order is not strictly required, but we mirror the proven-working
// reference to maximize the chance the client's typed handlers all fire. curHP/maxHP,
// coins (field 32 CUR_COIN — the buy-phase money the shop UI reads), and faction
// (field 34 FACTION_ID — 0=left/right gate team; the ONLY thing that puts an entity
// on the enemy team) vary; everything else is a sensible default. HP/MaxHP=200.
//
// score (field 28) is the CS round-win scoreboard: a u16 packed as
// (ownTeamWins) | (oppoTeamWins<<8) FROM THIS ENTITY'S team perspective. Changing it
// fires Player::PCNPOEKDIOC -> EventID_PLAYER_SCORE_CHANGED_CS, which drives the
// top-HUD myWinNum/oppoWinNum (the client re-resolves my/oppo from the entity's team).
// SetData is a no-op when unchanged, so the number only moves when the value actually
// changes. See PackScore + [[bot-networkaipawn]].
// PRIState is the per-player state a cmd-900 PRI block carries. Grouping the fields (this block grew
// to 14+ positional args) into a struct keeps the PRIHPBlock call readable and misorder-proof.
type PRIState struct {
	CurHP, MaxHP       uint16
	Coins, EarnedCoin  uint16
	Score              uint16 // packed own|oppo<<8 (PackScore)
	VestDur, HelmetDur uint16 // armor durability (fields 2/3)
	ItemOnHand         uint64 // (uniqueID<<32)|weaponDataID (field 4) — remote pawn's held weapon
	Faction            byte
	Kills, Deaths      byte
	FireState          byte   // START_FIRE_STATE (field 21) — remote muzzle/tracer
	Sighting           uint32 // IS_SIGHTING (field 12) — remote ADS/scoped pose
	CurEP              byte
	Damage             uint32
	SpeedScale         uint16 // GAMEMODE_SPEEDSCALE (field 50), percent (mul*100); 0 = omit (default 1.0x)
	JumpScale          uint16 // GAMEMODE_JUMPHEIGHTSCALE (field 52), percent (mul*100); 0 = omit
}

func PRIHPBlock(st PRIState) []byte {
	entries := []RepEntry{
		{RepU16, 0, uint64(st.CurHP)},     // CUR_HP
		{RepU16, 1, uint64(st.MaxHP)},     // MAX_HP
		{RepU64, 2, uint64(st.VestDur)},   // VEST_DURABILITY
		{RepU64, 3, uint64(st.HelmetDur)}, // HELMET_DURABILITY
		{RepU64, 4, st.ItemOnHand},        // ITEM_ON_HAND = (uniqueID<<32)|weaponDataID
		{RepU8, 5, 0},                     // IS_RESCURING
		{RepU8, 21, uint64(st.FireState)}, // START_FIRE_STATE (0 idle / 1 firing) — remote muzzle+tracer
		{RepU8, 6, uint64(st.CurEP)},      // CUR_EP
		{RepU8, 7, 200},                   // MAX_EP
		{RepU32, 8, 0},                    // CUR_AP
		{RepU32, 9, 0},                    // MAX_AP
		{RepU16, 39, 0},                   // (unknown, new)
		{RepU8, 10, 1},                    // LIFE_COUNT
		{RepU8, 11, 0},                    // STATUS
		{RepU32, 12, uint64(st.Sighting)}, // SIGHTING_ID
		{RepU8, 13, uint64(st.Kills)},     // CAMOUFLAGE_HP
		{RepU8, 14, 0},                    // OB_COUNT
		{RepU16, 15, 0},                   // BUFF
		{RepU64, 16, 0},                   // CAMOUFLAGE_HP
		{RepU8, 17, 0},                    // LIKE_COUNT
		{RepU64, 18, 0},
		{RepBool, 19, 0},
		{RepU32, 20, uint64(st.Kills)}, // PVE_KILL_COUNT
		{RepU16, 23, 0},                // CUR_HYPE
		{RepU8, 25, 0},                 // HYPE_LEVEL
		{RepU8, 24, 0},                 // MAX_HYPE_LEVEL
		{RepU16, 22, 0},                // MAX_HYPE
		{RepU32, 26, 0},
		{RepU64, 27, 0},
		{RepU16, 28, uint64(st.Score)},      // SCORE (CS round wins, packed own|oppo<<8)
		{RepU8, 29, uint64(st.Deaths)},      // DEAD_COUNT
		{RepU8, 30, uint64(st.Kills)},       // ASSIST_COUNT
		{RepU32, 31, uint64(st.Damage)},     // TOTAL_DAMAGE
		{RepU16, 32, uint64(st.Coins)},      // CUR_COIN (buy-phase money shown in the shop)
		{RepU16, 33, uint64(st.EarnedCoin)}, // EARNED_COIN
		{RepU8, 34, uint64(st.Faction)},     // FACTION_ID (0/1 = which gate team)
		{RepU32, 36, 0},                     // THROW_KNIFE_PHASE
		{RepU8, 37, 0},                      // TRAINING_ZONE_TYPE
		{RepU8, 38, 0},
	}
	// Custom-room move-speed / jump-height multipliers (percent). Emitted ONLY when set — the client
	// reads field 50/52 / 100 into PlayerAttributes (local + remote CS pawns); a 0 would freeze/kill
	// movement, so a default (1.0x) match simply omits the fields and keeps the client's own scale.
	if st.SpeedScale != 0 {
		entries = append(entries, RepEntry{RepU16, 50, uint64(st.SpeedScale)}) // GAMEMODE_SPEEDSCALE
	}
	if st.JumpScale != 0 {
		entries = append(entries, RepEntry{RepU16, 52, uint64(st.JumpScale)}) // GAMEMODE_JUMPHEIGHTSCALE
	}
	return ReplicationBlock(entries)
}

// PackScore packs a CS round-win pair into field 28's u16, FROM THE OWNING ENTITY'S
// team perspective: low byte = own team's wins, high byte = the opposing team's wins.
func PackScore(ownWins, oppoWins uint8) uint16 {
	return uint16(ownWins) | uint16(oppoWins)<<8
}
