package main

import (
	"log"
	"math/rand"

	"libmadoka/match-server/message"
	"libmadoka/match-server/token"
)

// Fallback player identity, used only when cmd 440 carries no valid prepare_token
// (e.g. a direct/manual test). Normally the player comes from the JWT prepare_token.
const (
	fallbackAccountID = 10000001
	fallbackName      = "foices"
	// playerEntityID follows the reference PlayerIDGenerator: a u32 whose HIBYTE
	// (bits 24-31) is the TEAM and whose low 24 bits are the slot. Team membership
	// (enemy vs teammate, nametags, damage) is keyed by the hibyte — two entities are
	// teammates iff they share a hibyte (get_hibyte(player_id) == team_index). The
	// local player is the first generated id: hibyte 1 (team 1), slot 1. It is
	// recognised as ours by IsLocalPlayer via ACCOUNT match (the id is not in {0,1}).
	playerEntityID = 0x01000001 // hibyte 1 (team 1), slot 1
	playerRepID    = 1000       // local player's PRI RepID (first EntityType=1 BindPRI entry; props/vehicles use other ranges)
	localFaction   = 0          // local player's CS FACTION_ID (0=left gate); matches PRIHPBlock field 34
	maxHP          = 200        // starting/max HP for a player entity (PRI fields 0/1)
	// defaultBattleFlag equips every player a battle flag (BattleFlag.csv 1410000001 = Pirate) so flag emotes
	// plant — the client only sends cmd 0x114 when BattleFlagID != 0. It's a testing default; the JWT's
	// battle_flag should carry the player's real equipped flag. Any nonzero value works (the flag model
	// resolves from the emote link_id, not this id). See [[cs-emotes]].
	defaultBattleFlag = 1410000001
)

// Enemy bot identity. RE showed a CS enemy is just an ordinary remote Player entity
// (cmd 101 join + cmd 118 BindPRI + cmd 900 PRI). Its TEAM is decided by the HIBYTE of
// its entity/player id (the reference PlayerIDGenerator scheme), NOT by FACTION_ID
// alone — a naive EntityID=2 shares hibyte 0 with the local player (EntityID=1) and
// renders as a TEAMMATE. So the bot uses the second generated id: hibyte 2 (team 2),
// a DIFFERENT team from the local player's hibyte 1 -> enemy. Movement/fire must be
// server-driven (bots run no client AI); this first stage is a static dummy standing
// at the opposing gate. See [[bot-networkaipawn]].
const (
	// botAccountID must NOT equal any human/guest account id: the client's IsLocalPlayer picks its
	// own pawn by ACCOUNT match, so a bot sharing a player's account makes that player's client
	// adopt the BOT as its local entity. Hit live — guest "matheus" = 10000002 = the old bot id —
	// so it is kept far above the sequential guest range (10000001, 10000002, ...).
	botAccountID = 900000002
	botName      = "BOT"
	botEntityID  = 0x02000002 // hibyte 2 (team 2), slot 2 -> different team from local (hibyte 1) = enemy
	botRepID     = 1001       // bot's PRI RepID (second EntityType=1 BindPRI entry, after the local player)
	botFaction   = 1          // CS FACTION_ID = right gate (spawn side); the team split is the hibyte above
)

// botSpawnOffset is how many metres in front of the local player the bot spawns —
// kept close so the player can fight it (per the round design), not across the map.
const botSpawnOffset = 2.0

// botPlayer builds the enemy bot's identity at the given entity id: same cosmetics as
// the local player (so it renders a body) but a distinct account/name, spawned
// botSpawnOffset metres in front of the local player and facing back at them.
func botPlayer(local joinPlayer, entity uint32) joinPlayer {
	bot := joinPlayer{}
	bot.AccountID = botAccountID
	bot.EntityID = entity
	bot.Name = botName
	bot.SpawnPos = message.Vec3{
		X: local.SpawnPos.X + local.SpawnFace.X*botSpawnOffset,
		Y: local.SpawnPos.Y,
		Z: local.SpawnPos.Z + local.SpawnFace.Z*botSpawnOffset,
	}
	bot.SpawnFace = faceToward(bot.SpawnPos, local.SpawnPos)
	return bot
}

// joinPlayer is the identity + cosmetics used to build cmd 101 PLAYER_JOIN. The
// underlying type lives in the message package (as PlayerInfo) so the serializers
// can consume it — a main-package type can't be imported by other packages — and
// is aliased here for readability.
type joinPlayer = message.PlayerInfo

// resolvePlayer verifies the cmd 440 prepare_token and returns the player it
// describes (identity + cosmetics from the JWT `show` claim). A missing/invalid
// token falls back to the stub player so a manual join still works.
func resolvePlayer(tok string) joinPlayer {
	p := joinPlayer{AccountID: fallbackAccountID, EntityID: playerEntityID, Name: fallbackName}
	p.BattleFlag = defaultBattleFlag // equip a flag so flag emotes plant; the JWT overrides below if it carries one
	if tok == "" {
		log.Printf("[mm-udp] cmd 440 carried no prepare_token — using fallback player %d %q", p.AccountID, p.Name)
		return p
	}
	claims, err := token.Verify(tok, cfg.jwtSecret)
	if err != nil {
		log.Printf("[mm-udp] prepare_token invalid (%v) — using fallback player %d %q", err, p.AccountID, p.Name)
		return p
	}
	if claims.AccountID != 0 {
		p.AccountID = claims.AccountID
	}
	p.MatchID = claims.MatchID // shared match id (settlement idempotency key)
	if claims.Role != 0 {
		p.Role = claims.Role
	}
	if claims.Name != "" {
		p.Name = claims.Name
	}
	if s := claims.Show; s != nil {
		p.Avatar, p.Color, p.Head, p.Banner = s.Avatar, s.Color, s.Head, s.Banner
		p.Clothes, p.Slots = s.Clothes, s.Slots
		p.Emotes = s.Emotes
		if s.BattleFlag != 0 { // the player's real equipped flag, if the token carries one
			p.BattleFlag = s.BattleFlag
		}
	}
	log.Printf("[mm-udp] prepare_token OK: acc=%d name=%q region=%q role=%d match=%d avatar=%d clothes=%d slots=%d emotes=%d",
		p.AccountID, p.Name, claims.Region, p.Role, claims.MatchID, p.Avatar, len(p.Clothes), len(p.Slots), len(p.Emotes))
	return p
}

// choosePlayerSpawn sets pl's authoritative spawn transform (cmd 101
// OGJKHJAFNHB.CCIKDFGDBAM/CCDDHEBKMGD): a fixed MATCH_SPAWN position for testing, or
// a random CS city's fence for the local faction, facing the opposing fence. Returns
// the chosen arena (empty for the fixed override) so the session can remember it.
func choosePlayerSpawn(pl *joinPlayer, used map[csArena]bool) csArena {
	if cfg.spawn != nil {
		pl.SpawnPos, pl.SpawnFace = *cfg.spawn, cfg.spawnFace
		log.Printf("[mm-udp] SPAWN override: pos=(%.2f,%.2f,%.2f) face=(%.2f,%.2f,%.2f)",
			pl.SpawnPos.X, pl.SpawnPos.Y, pl.SpawnPos.Z,
			pl.SpawnFace.X, pl.SpawnFace.Y, pl.SpawnFace.Z)
		return csArena{}
	}

	var available []csArena
	for _, arena := range allArenas() {
		if !used[arena] {
			available = append(available, arena)
		}
	}

	// Já utilizou todas as arenas.
	if len(available) == 0 {
		clear(used) // ou delete() em loop se Go < 1.21
		available = append(available, allArenas()...)
	}

	arena := available[rand.Intn(len(available))]

	pl.SpawnPos, pl.SpawnFace = arena.spawnFor(localFaction)
	log.Printf("[mm-udp] CS spawn: city=%q faction=%d pos=(%.1f,%.1f,%.1f) face=(%.2f,%.2f,%.2f)",
		arena.City, localFaction,
		pl.SpawnPos.X, pl.SpawnPos.Y, pl.SpawnPos.Z,
		pl.SpawnFace.X, pl.SpawnFace.Y, pl.SpawnFace.Z)

	return arena
}
