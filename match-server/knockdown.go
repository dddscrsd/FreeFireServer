package main

import (
	"time"

	"libmadoka/match-server/message"
)

// Multiplayer death / knockdown rules for Contra Squad teams (Step 5). When a team has more than
// one player, a lethal hit KNOCKS a player down (revivable by a teammate) instead of killing them;
// they bleed out over ~30s, and the round ends when a whole team is eliminated. A 1-per-team match
// (today's human vs bot) keeps the solo pending-revive + cmd 388 path — see combat.go / round.go.
//
// This file is the RE-independent foundation: the lifecycle model + team helpers. The transitions
// (knock / bleed / revive / force-eliminate) and the exact downed-PRI + revive wire land once the
// client's DBNO handler is confirmed.
const (
	knockdownHP     = 200                     // bleed pool a knocked player starts with
	knockBleedAmt   = 10                      // HP lost per bleed tick
	knockBleedEvery = 1500 * time.Millisecond // bleed cadence (knockdownHP/knockBleedAmt ticks ~= 30s window)
	reviveHP        = 30                      // HP a mid-round teammate revive restores (round transition restores full)
)

// lifeState is a participant's per-round lifecycle.
type lifeState uint8

const (
	lifeAlive   lifeState = iota // up and fighting; takes normal HP damage
	lifeKnocked                  // downed + bleeding; revivable by a teammate; not yet a cmd 107
	lifeDead                     // eliminated (cmd 107 sent); out until the round transition
)

// knockState is the live record for a KNOCKED player: the bleed pool + the death info resolved at
// knock time (from the KILLER's loadout), so the eventual cmd 107 carries the right weapon/killer.
type knockState struct {
	hp         uint16    // bleed pool: starts knockdownHP, -knockBleedAmt each tick; 0 -> death
	deadline   time.Time // next bleed tick
	killer     uint32    // recorded at knock time:
	weapon     int32
	weaponSkin int32
	bodyPart   uint8
	pos        message.Vec3
}

// lifeOf returns an entity's lifecycle (absent == lifeAlive).
func (m *Match) lifeOf(entity uint32) lifeState { return m.life[entity] }

// teamOf returns the team (entity-id hibyte) an entity belongs to.
func (m *Match) teamOf(entity uint32) byte { return byte(entity >> 24) }

// sessionByEntity finds the roster human with the given entity id, or nil (e.g. the bot).
func (m *Match) sessionByEntity(entity uint32) *session {
	for _, p := range m.players {
		if p.entityID == entity {
			return p
		}
	}
	return nil
}

// teamSize counts a team's participants: roster humans plus the bot (which is on the enemy team).
func (m *Match) teamSize(team byte) int {
	n := 0
	for _, p := range m.players {
		if p.team == team {
			n++
		}
	}
	if team == enemyTeamID && m.botEntity != 0 {
		n++
	}
	return n
}

// teamAlive counts a team's ALIVE participants (roster humans + the bot).
func (m *Match) teamAlive(team byte) int {
	n := 0
	for _, p := range m.players {
		if p.team == team && m.lifeOf(p.entityID) == lifeAlive {
			n++
		}
	}
	if team == enemyTeamID && m.botEntity != 0 && m.lifeOf(m.botEntity) == lifeAlive {
		n++
	}
	return n
}

// resolveKillWeapon returns the (weapon, skin) to tag a death with, taken from the killer's held
// loadout; killer 0 is an out-of-zone environment death (KILL_BY_DAMAGEZONE).
func (m *Match) resolveKillWeapon(killer uint32) (weapon, skin int32) {
	if killer == 0 {
		return killCauseDamageZone, 0
	}
	if ks := m.sessionByEntity(killer); ks != nil {
		w := int32(ks.heldWeaponData())
		if w == 0 {
			w = 1 // fists
		}
		return w, int32(SkinForWeapon(uint32(w), []uint32(ks.player.Slots)))
	}
	return 1, 0 // unknown killer (e.g. the bot) -> fists, no skin
}
