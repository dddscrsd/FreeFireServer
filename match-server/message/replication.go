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

// EMatchDrawState (GRI field 5, high 16 bits). NormalStart is required for normal
// play; HalfwayJoin shows a "waiting to connect" countdown instead.
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

// CSGRIInit builds the CS GRI block: round config (max + current round, 0-based) +
// NormalStart draw state. Streamed continuously; bump currentRound to advance rounds.
func CSGRIInit(maxRound, currentRound uint8) []byte {
	return ReplicationBlock([]RepEntry{
		{RepU8, CSFieldMaxRound, uint64(maxRound)},
		{RepU8, CSFieldCurrentRound, uint64(currentRound)},
		{RepU8, CSFieldMatchPoint, 0},
		{RepU32, CSFieldDrawState, packPhase(CSDrawNormalStart, 0)},
	})
}

// CSGRIPhase builds a GRI block that sets only the match phase (field 3). param
// is the phase's lo16 (buy/round end time for the client countdown; 0 = none).
func CSGRIPhase(phase, param uint16) []byte {
	return ReplicationBlock([]RepEntry{
		{RepU32, CSFieldPhase, packPhase(phase, param)},
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

// BindPRI entity types.
const (
	BindEntityPlayer  byte = 1
	BindEntityVehicle byte = 3
	BindEntityOilDrum byte = 5
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

// SyncPRI builds the cmd 500 payload: per entity [RepID u32][outerLen u16][block]
// (the entity wrapper adds an outer length around the block's own blockLen).
func SyncPRI(entities []PRIEntity) []byte {
	w := &Writer{}
	for _, e := range entities {
		w.U32(e.RepID)
		w.U16(uint16(len(e.Block)))
		w.B = append(w.B, e.Block...)
	}
	return w.B
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
func PRIHPBlock(curHP, maxHP, coins, score uint16, faction byte) []byte {
	return ReplicationBlock([]RepEntry{
		{RepU16, 0, uint64(curHP)}, // CUR_HP
		{RepU16, 1, uint64(maxHP)}, // MAX_HP
		{RepU64, 2, 0},             // VEST_DURABILITY
		{RepU64, 3, 0},             // HELMET_DURABILITY
		{RepU64, 4, 0},             // ITEM_ON_HAND
		{RepU8, 5, 0},              // IS_RESCURING
		{RepU8, 21, 0},             // START_FIRE_STATE
		{RepU8, 6, 0},              // CUR_EP
		{RepU8, 7, 200},            // MAX_EP
		{RepU32, 8, 0},             // STATUS
		{RepU16, 39, 0},            // (unknown, new)
		{RepU32, 9, 0},             // SIGHTING_ID
		{RepU8, 10, 0},             // KILL_COUNT
		{RepU8, 11, 0},             // OB_COUNT
		{RepU32, 12, 0},            // BUFF
		{RepU8, 13, 0},             // CAMOUFLAGE_HP
		{RepU8, 14, 0},             // LIKED_COUNT
		{RepU16, 15, 0},
		{RepU64, 16, 0},
		{RepU8, 17, 0},
		{RepU64, 18, 0},
		{RepBool, 19, 0},
		{RepU32, 20, 0}, // PVE_KILL_COUNT
		{RepU16, 23, 0}, // CUR_HYPE
		{RepU8, 25, 0},  // HYPE_LEVEL
		{RepU8, 24, 0},  // MAX_HYPE_LEVEL
		{RepU16, 22, 0}, // MAX_HYPE
		{RepU32, 26, 0},
		{RepU64, 27, 0},
		{RepU16, 28, uint64(score)},  // SCORE (CS round wins, packed own|oppo<<8)
		{RepU8, 29, 0},               // DEAD_COUNT
		{RepU8, 30, 0},               // ASSIST_COUNT
		{RepU32, 31, 0},              // TOTAL_DAMAGE
		{RepU16, 32, uint64(coins)},  // CUR_COIN (buy-phase money shown in the shop)
		{RepU16, 33, 0},              // EARNED_COIN
		{RepU8, 34, uint64(faction)}, // FACTION_ID (0/1 = which gate team)
		{RepU32, 36, 0},              // THROW_KNIFE_PHASE
		{RepU8, 37, 0},               // TRAINING_ZONE_TYPE
		{RepU8, 38, 0},
	})
}

// PackScore packs a CS round-win pair into field 28's u16, FROM THE OWNING ENTITY'S
// team perspective: low byte = own team's wins, high byte = the opposing team's wins.
func PackScore(ownWins, oppoWins uint8) uint16 {
	return uint16(ownWins) | uint16(oppoWins)<<8
}
