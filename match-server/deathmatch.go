package main

import (
	"fmt"
	"log"
	"time"

	"libmadoka/match-server/message"
	"libmadoka/match-server/packet"
)

// deathmatch.go is the multiplayer death state machine (Step 5 / "B2"): damage -> knock-or-kill ->
// bleed -> round-end, driven off the team helpers in knockdown.go. The 1-per-team case (the bot, and
// today's solo-human 1v1) keeps the exact cmd-107 + pending-revive path; a team with a backup member
// gets the DBNO knockdown (cmd 139) + bleed-out instead. All events fan out to the whole roster.

// broadcastData reliably fans one data event (death, knock, revive) out to EVERY roster client. Each
// connection's Writer stamps its own seq/order, so a single encoded payload frames correctly per
// client. (The 1v1 paths used to send only to the reporting client, which is fine for one human.)
func (m *Match) broadcastData(cmd uint16, payload []byte, what string) {
	for _, p := range m.players {
		if p.out != nil {
			p.out.send(&packet.Packet{SendOption: packet.SendReliable, Cmd: cmd, Flags: packet.FlagEncrypted, Payload: payload}, what)
		}
	}
}

func (m *Match) broadcastDataUnreliable(cmd uint16, payload []byte, what string) {
	for _, p := range m.players {
		if p.out != nil {
			p.out.send(&packet.Packet{SendOption: packet.SendUnreliable, Cmd: cmd, Flags: packet.FlagEncrypted, Payload: payload}, what)
		}
	}
}

// deathPos returns the world position to report an entity died / was knocked at: a human's
// last-known position (it moves — e.g. a zone death where it wandered), or the bot's static spawn.
func (m *Match) deathPos(entity uint32) message.Vec3 {
	if m.botEntity != 0 && entity == m.botEntity {
		return m.bot.SpawnPos
	}
	if vs := m.sessionByEntity(entity); vs != nil {
		if vs.playerPos != (message.Vec3{}) {
			return vs.playerPos
		}
		return vs.player.SpawnPos
	}
	return message.Vec3{}
}

// applyDamage subtracts dmg from the victim's HP (clamped at 0) and, on the transition to 0, decides
// the outcome (downOrKill). A knocked/dead victim or a decided round ignores further damage. attacker
// 0 is an environment (out-of-zone) hit.
func (m *Match) applyDamage(victim, attacker uint32, dmg uint16, bodyPart uint8) {
	if m.roundOver {
		return
	}
	if m.lifeOf(victim) == lifeKnocked {
		m.damageKnocked(victim, attacker, dmg) // downed players still take real damage (finish / zone)
		return
	}
	if m.lifeOf(victim) != lifeAlive {
		return
	}
	cur, ok := m.hp[victim]
	if !ok || cur == 0 {
		return
	}
	if dmg >= cur {
		cur = 0
	} else {
		cur -= dmg
	}
	m.hp[victim] = cur
	weapon, _ := m.resolveKillWeapon(attacker) // for the hit-direction indicator + spectator relays
	if attacker != 0 && m.teamOf(attacker) != m.teamOf(victim) {
		m.damage[attacker] += uint32(dmg) // scoreboard total damage (PRI field 31)
		// Floating damage numbers (cmd 168). The shooter renders its own locally, so send it to the
		// SHOOTER too (suppressed live via the own-shooter gate) — that puts it in the shooter's replay
		// RECORDING, where the handler's replay branch shows it. Also relayed to spectators of the
		// shooter (a dead teammate watching them). Not sent to unrelated players (it's the shooter's UI).
		// A head hit (bodyPart == bodyHead) flags it as a headshot so the number renders RED (critical) —
		// the shooter's own local render knows this, but a spectator/replay only learns it from here.
		dmgBody := message.ShowDamage(attacker, victim, dmg, uint32(weapon), bodyPart == bodyHead)
		if as := m.sessionByEntity(attacker); as != nil {
			as.sendData(packet.CmdShowDamage, dmgBody)
		}
		for _, sp := range m.spectatorsOf(attacker) {
			sp.sendData(packet.CmdShowDamage, dmgBody)
		}
	}
	if vs := m.sessionByEntity(victim); vs != nil {
		vs.wearArmor(bodyPart, dmg) // client already reduced HP; we only track + stream the armor wear
		// Forward a cmd-106 TakeDamage to the VICTIM so its client renders the hit-direction indicator
		// (which way the shot came from).
		vs.sendData(packet.CmdTakeDamage, message.TakeDamage(victim, attacker, uint32(weapon), dmg, int8(bodyPart)))
		vs.sendVar(packet.CmdPRISync, vs.match.priPayload(), 1)
	}
	// A spectator watching the VICTIM also renders the hurt-direction arrow — the client orients it off
	// the observed pawn (GetLocalPlayerOrObServer), so relay the same cmd 106 to each spectator.
	for _, sp := range m.spectatorsOf(victim) {
		sp.sendData(packet.CmdTakeDamage, message.TakeDamage(victim, attacker, uint32(weapon), dmg, int8(bodyPart)))
	}
	// Hit-reaction: tell EVERYONE to play the victim's flinch (cmd 1010) on this hit — the client
	// self-throttles + gates it out when the victim is dying. No-attacker (zone) damage passes the
	// victim as the source, which the client renders as the default blood + flinch (no weapon lookup).
	src := attacker
	if src == 0 {
		src = victim
	}
	m.broadcastData(packet.CmdHurtAnim, message.HurtAnim(victim, src),
		fmt.Sprintf("cmd=1010 hurt victim=%#x src=%#x", victim, src))
	log.Printf("[mm-udp] TAKE_DAMAGE victim=%#x dmg=%d -> HP=%d", victim, dmg, cur)
	if cur == 0 {
		m.downOrKill(victim, attacker, bodyPart)
	}
}

// damageKnocked drains a KNOCKED player's bleed pool by EXTERNAL damage — enemy fire finishing a
// downed target, or the safe zone — which the alive-path applyDamage would otherwise ignore (it early-
// returns for non-alive victims, so a downed player only ever lost HP to the bleed timer). The pool
// (ks.hp, mirrored into m.hp for the PRI stream) drops by dmg; emptying it finalizes the knock into a
// cmd-107 death, crediting the original knocker (ks.killer). attacker 0 = zone/environment.
func (m *Match) damageKnocked(victim, attacker uint32, dmg uint16) {
	ks, ok := m.knock[victim]
	if !ok {
		return
	}
	if attacker != 0 && m.teamOf(attacker) != m.teamOf(victim) {
		m.damage[attacker] += uint32(dmg) // scoreboard total damage (PRI field 31)
	}
	if dmg >= ks.hp {
		ks.hp = 0
		m.hp[victim] = 0
		m.finalizeKnock(victim, ks)
		return
	}
	ks.hp -= dmg
	m.hp[victim] = ks.hp // PRI streams the draining bleed pool as the downed player's HP
	if vs := m.sessionByEntity(victim); vs != nil {
		vs.sendVar(packet.CmdPRISync, vs.match.priPayload(), 1) // eager: show the downed HP drop now
	}
}

// downOrKill resolves a lethal hit. On a team that still has a backup member the victim is KNOCKED
// (cmd 139, revivable); a 1-per-team side (the bot / the solo-human 1v1) takes the immediate cmd-107
// path. A knock that empties the team force-eliminates the whole team.
func (m *Match) downOrKill(victim, attacker uint32, bodyPart uint8) {
	team := m.teamOf(victim)
	if m.teamSize(team) <= 1 {
		m.killEntity(victim, attacker, bodyPart)
		return
	}
	m.knockPlayer(victim, attacker, bodyPart)
	if m.teamAlive(team) == 0 {
		m.teamFellToZero(team) // the last-standing member went down -> the whole team is out
	}
}

// knockPlayer downs the victim (cmd 139): still ALIVE, crawling + bleeding, revivable by a teammate.
// The bleed pool + the death info (resolved from the KILLER's loadout now) are recorded so the
// eventual cmd 107 is tagged correctly. m.hp holds the bleed pool so the PRI stream shows it draining.
func (m *Match) knockPlayer(victim, attacker uint32, bodyPart uint8) {
	weapon, skin := m.resolveKillWeapon(attacker)
	m.life[victim] = lifeKnocked
	m.knock[victim] = &knockState{
		hp:         knockdownHP,
		deadline:   time.Now().Add(knockBleedEvery),
		killer:     attacker,
		weapon:     weapon,
		weaponSkin: skin,
		bodyPart:   bodyPart,
		pos:        m.deathPos(victim),
	}
	m.hp[victim] = knockdownHP
	m.broadcastData(packet.CmdKnockDown,
		message.KnockDown(victim, attacker, uint32(weapon), uint32(skin), bodyPart, bodyPart == 1),
		fmt.Sprintf("cmd=139 KNOCK victim=%#x knocker=%#x weapon=%d team=%d alive-left=%d", victim, attacker, weapon, m.teamOf(victim), m.teamAlive(m.teamOf(victim))))
}

// finalizeKnock turns a KNOCKED player into a real cmd-107 death using the recorded knock info. Used
// by the bleed-out timer (stepKnock) and by teamFellToZero (team wipe).
func (m *Match) finalizeKnock(entity uint32, ks *knockState) {
	m.creditDeath(entity, ks.killer)
	m.life[entity] = lifeDead
	m.hp[entity] = 0
	delete(m.knock, entity)
	delete(m.rescues, entity) // cancel any in-progress revive of the now-dead player
	// a human keeps its pawn (pending revive) for the round-transition revive-all; observe = own
	// corpse so the client stays in the match (un-spectated when the round resets it).
	pendingRevive := m.sessionByEntity(entity) != nil
	var observe uint32
	if pendingRevive {
		observe = m.spectateFor(entity) // watch a live teammate (or an enemy at round-end), not own corpse
	}
	m.emitDeath(entity, ks.killer, uint32(ks.weapon), uint32(ks.weaponSkin), observe, ks.bodyPart, ks.pos, false, pendingRevive)
}

// creditDeath records a death for the scoreboard: +1 death for the victim, and +1 kill (plus the
// per-round coin credit) for the killer if it was an enemy. killer 0 = environment (zone) -> no kill.
func (m *Match) creditDeath(victim, killer uint32) {
	m.deaths[victim]++
	if killer != 0 && m.teamOf(killer) != m.teamOf(victim) {
		m.kills[killer]++
		if m.firstBlood == 0 {
			m.firstBlood = killer // first cross-team kill of the round -> the CS first-blood bonus (awarded in endFight)
		}
		if ks := m.sessionByEntity(killer); ks != nil {
			ks.roundKills.Add(1)
		}
	}
}

// teamFellToZero force-eliminates every still-knocked member of a wiped team (cmd 107 each), so the
// pending knocks become real deaths for the round-end + kill feed.
func (m *Match) teamFellToZero(team byte) {
	for e, ks := range m.knock {
		if m.teamOf(e) == team && m.lifeOf(e) == lifeKnocked {
			m.finalizeKnock(e, ks)
		}
	}
	m.lastTeamFell = team
}

// killEntity is the immediate-death (1-per-team) path: mark victim dead and broadcast cmd 107. A
// human killer credits a round-kill for an enemy victim; a human victim keeps its pawn (pending
// revive) + a spectate focus so the 1v1 round-reset (cmd 388) can resurrect it.
func (m *Match) killEntity(victim, killer uint32, bodyPart uint8) {
	m.creditDeath(victim, killer)
	weapon, skin := m.resolveKillWeapon(killer)
	m.life[victim] = lifeDead
	m.hp[victim] = 0
	pendingRevive := m.sessionByEntity(victim) != nil
	var observe uint32
	if pendingRevive {
		observe = m.spectateFor(victim)
	} else {
		observe = 0 // the bot or a match-ending death -> no spectate focus
	}
	m.emitDeath(victim, killer, uint32(weapon), uint32(skin), observe, bodyPart, m.deathPos(victim), false, pendingRevive)
}

// emitDeath broadcasts one cmd 107 (entity died) to the whole match. pendingRevive keeps a human's
// pawn (down) for the round-transition revive; observe is the dead client's spectate focus (0 = none,
// e.g. the bot or a match-ending death).
func (m *Match) emitDeath(victim, killer, weapon, weaponSkin, observe uint32, bodyPart uint8, pos message.Vec3, system, pendingRevive bool) {
	body := message.PlayerDead(victim, killer, weapon, weaponSkin, observe, bodyPart, pos, system, pendingRevive)
	m.broadcastData(packet.CmdDead, body,
		fmt.Sprintf("cmd=107 DEAD victim=%#x killer=%#x weapon=%d observe=%#x pending=%v", victim, killer, weapon, observe, pendingRevive))

	// The dead client now spectates `observe`; its camera re-renders that pawn's weapon, which can drop the
	// mounted attachments (e.g. a 2x scope reverts to the default 1x on the spectator's view). Re-send the
	// watched player's attachment mounts (cmd 124) so the spectator sees the same maxed attachments. Guarded:
	// `observe` may be the bot (no session) and the victim may be the bot too.
	if observe != 0 {
		if spectator := m.sessionByEntity(victim); spectator != nil {
			spectator.observing = observe // remember who this dead client watches (hit/damage relay + OB_COUNT)
			if watched := m.sessionByEntity(observe); watched != nil {
				watched.resendEquipTo(spectator)  // re-affirm the watched player's back-mounted weapons
				watched.resendAttachTo(spectator) // + attachments (so a 2x scope isn't seen as the default 1x)
			}
		}
		// Anyone who was already spectating THIS now-dead player follows the chain to its new focus, so
		// their OB_COUNT + hit/damage relays track the live pawn instead of the corpse.
		m.repointSpectators(victim, observe)
	}
}

// repointSpectators moves every session currently spectating `gone` to a fresh live target and tells
// each client authoritatively via cmd 149 (RUDP_SWITCH_OBSERVE). `prefer` is the follow-the-chain
// target (the dead player's own new focus); if it's 0 or no longer alive, each spectator falls back to
// a live teammate of its own (spectateFor). Called when a watched player dies (emitDeath) or quits
// (removePlayer) so the spectator experience — OB_COUNT badge + hit-marker/damage-number relays —
// keeps following whoever the client is actually watching.
func (m *Match) repointSpectators(gone, prefer uint32) {
	for _, sp := range m.spectatorsOf(gone) {
		target := prefer
		if target == 0 || m.lifeOf(target) != lifeAlive {
			target = m.spectateFor(sp.entityID)
		}
		sp.observing = target
		if target != 0 {
			sp.sendData(packet.CmdSwitchObserve, message.SwitchObserve(target))
		}
	}
}

// spectatorsOf returns the live sessions currently spectating `entity` (dead teammates watching it).
// Used to relay the watched pawn's hit-direction (cmd 106) + damage numbers (cmd 168) to them.
func (m *Match) spectatorsOf(entity uint32) []*session {
	var out []*session
	for _, p := range m.players {
		if p.observing == entity {
			out = append(out, p)
		}
	}
	return out
}

// obCount is how many sessions are spectating `entity` — the OB_COUNT (PRI field 14, u8) badge above
// that player. Capped at 255.
func (m *Match) obCount(entity uint32) byte {
	n := 0
	for _, p := range m.players {
		if p.observing == entity {
			n++
		}
	}
	if n > 255 {
		n = 255
	}
	return byte(n)
}

// stepKnock bleeds every KNOCKED player on run()'s tick: -knockBleedAmt on the cadence; at 0 the
// knock finalizes into a real cmd-107 death (which the phase machine then reads for the round-end).
func (m *Match) stepKnock(now time.Time) {
	for e, ks := range m.knock {
		if now.Before(ks.deadline) {
			continue
		}
		ks.deadline = ks.deadline.Add(knockBleedEvery)
		if ks.hp <= knockBleedAmt {
			m.finalizeKnock(e, ks)
			continue
		}
		ks.hp -= knockBleedAmt
		m.hp[e] = ks.hp // PRI streams the draining bleed pool as the downed player's HP
	}
}

// reviveAll resets every roster human + the bot to full HP + ALIVE and clears all knocks — the
// round-transition wipe, so a new round starts everyone up. Clears roundOver so damage resumes.
func (m *Match) reviveAll() {
	for _, p := range m.players {
		m.life[p.entityID] = lifeAlive
		m.hp[p.entityID] = m.settings.maxHP
		p.observing = 0 // back alive -> no longer spectating (clears OB_COUNT + the relays)
	}
	if m.botEntity != 0 {
		m.life[m.botEntity] = lifeAlive
		m.hp[m.botEntity] = m.settings.maxHP
	}
	m.knock = map[uint32]*knockState{}
	m.rescues = map[uint32]*rescueState{}
	m.roundOver = false
}

// spectateFor picks a dead player's spectate focus: a live TEAMMATE if any, else a live enemy (the
// team was wiped — round end), else 0. Keeps the dead client in the match with a valid camera until
// the round-transition revive.
func (m *Match) spectateFor(victim uint32) uint32 {
	team := m.teamOf(victim)
	if e := m.firstAlive(team, victim); e != 0 {
		return e
	}
	return m.firstAlive(otherTeam(team), victim)
}

// firstAlive returns any ALIVE participant of `team` other than `except` (a roster human, or the bot
// on the enemy team), or 0 if none is alive.
func (m *Match) firstAlive(team byte, except uint32) uint32 {
	for _, p := range m.players {
		if p.entityID != except && p.team == team && m.lifeOf(p.entityID) == lifeAlive {
			return p.entityID
		}
	}
	if team == enemyTeamID && m.botEntity != 0 && m.botEntity != except && m.lifeOf(m.botEntity) == lifeAlive {
		return m.botEntity
	}
	return 0
}

// otherTeam returns the opposing team id (1<->2).
func otherTeam(team byte) byte { return 3 - team }

// deadHumans returns the roster humans that are NOT alive (dead or still knocked) — captured before a
// reviveAll so the round transition knows who to resurrect with cmd 388.
func (m *Match) deadHumans() []*session {
	var dead []*session
	for _, p := range m.players {
		if m.lifeOf(p.entityID) != lifeAlive {
			dead = append(dead, p)
		}
	}
	return dead
}

// respawnPlayer resurrects one dead human via cmd 388 (NOTIFYREVIVE) at its spawn, broadcast so every
// client clears the pawn's dead flag + un-spectates (the dead client was watching a teammate). HP
// comes from the still-bound PRI stream. Generalises the old 1v1 respawnLocalPlayer.
func (m *Match) respawnPlayer(p *session) {
	p.observing = 0 // cmd 388 un-spectates the client; drop the server-side focus to match
	yaw := yawByte(p.player.SpawnFace)
	m.broadcastData(packet.CmdNotifyRevive, message.NotifyRevive(p.entityID, p.player.SpawnPos, yaw),
		fmt.Sprintf("cmd=388 revive ent=%#x pos=(%.1f,%.1f,%.1f) yaw=%d", p.entityID, p.player.SpawnPos.X, p.player.SpawnPos.Y, p.player.SpawnPos.Z, yaw))
}

// --- Teammate revive (DBNO rescue): cmd 142 START_RESCURE / 143 STOP_RESCURE -> cmd 140 KNOCK_REVIVE.
// A LIVING teammate holds-to-revive a KNOCKED player; after reviveDuration the target is un-knocked at
// reviveHP. (The DCBAMPDIHIG progress-bar ack is pending an opcode RE; the revive works without it.)

// rescueState is an in-progress teammate revive, keyed in m.rescues by the target (knocked) entity.
type rescueState struct {
	reviver, target uint32
	startMs         uint64    // client's cmd-142 startMs (for the eventual DCBAMPDIHIG progress bar)
	completeAt      time.Time // server-clock time the revive finishes
}

// startRescue begins a teammate revive of a knocked target (cmd 142). Ignored unless the target is
// KNOCKED and the reviver is a LIVING teammate.
func (m *Match) startRescue(reviver, target uint32, startMs uint64) {
	if m.lifeOf(target) != lifeKnocked || m.lifeOf(reviver) != lifeAlive || m.teamOf(reviver) != m.teamOf(target) {
		return
	}
	if _, active := m.rescues[target]; active {
		return // already being revived — don't reset the timer or re-ack
	}
	endMs := startMs + uint64(reviveDuration.Milliseconds())
	m.rescues[target] = &rescueState{reviver: reviver, target: target, startMs: startMs, completeAt: time.Now().Add(reviveDuration)}

	// cmd 142 is bidirectional: the client SENT START_RESCURE; we reply with the DCBAMPDIHIG ack on the
	// SAME opcode, which drives the revive progress bar over (endMs-startMs).
	m.broadcastData(packet.CmdStartRescue, message.RescueProgress(true, reviver, target, startMs, endMs, 0),
		fmt.Sprintf("cmd=142 RESCUE-ACK reviver=%#x target=%#x end=%dms", reviver, target, endMs))
	log.Printf("[mm-udp] RESCUE start reviver=%#x target=%#x", reviver, target)
}

// stopRescue cancels an in-progress revive (cmd 143 — the reviver moved away / released the button).
func (m *Match) stopRescue(reviver, target uint32) {
	if r, ok := m.rescues[target]; ok && r.reviver == reviver {
		delete(m.rescues, target)
		log.Printf("[mm-udp] RESCUE stop reviver=%#x target=%#x", reviver, target)
	}
}

// stepRescue runs each tick: complete a revive whose timer elapsed, and drop any whose target is no
// longer knocked (bled out / round reset) or whose reviver is no longer alive.
func (m *Match) stepRescue(now time.Time) {
	for target, r := range m.rescues {
		if m.lifeOf(target) != lifeKnocked || m.lifeOf(r.reviver) != lifeAlive {
			delete(m.rescues, target)
			continue
		}
		if now.Before(r.completeAt) {
			continue
		}
		delete(m.rescues, target)
		m.completeRescue(r)
	}
}

// completeRescue un-knocks a target at reviveHP and broadcasts cmd 140 so every client un-crawls it.
func (m *Match) completeRescue(r *rescueState) {
	m.life[r.target] = lifeAlive
	m.hp[r.target] = reviveHP
	delete(m.knock, r.target)
	m.broadcastData(packet.CmdKnockRevive, message.KnockRevive(r.target, r.reviver),
		fmt.Sprintf("cmd=140 REVIVE target=%#x reviver=%#x -> %d HP", r.target, r.reviver, reviveHP))
}
