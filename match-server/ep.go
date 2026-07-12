package main

import (
	"time"

	"libmadoka/match-server/packet"
)

// Shop-mushroom EP (energy). Buying a mushroom grants epPerMushroom energy (capped at maxEP), limited
// to mushroomsPerRound purchases per round; stepEP then drains that energy into HP over time (1 HP per
// epInterval == 4 HP/s, matching the reference server's _do_ep_regen), so a mushroom is a slow heal
// buffer streamed as PRI CUR_EP (field 6). EP + the per-round mushroom count reset each round (resetEP).
const (
	epPerMushroom     = 100
	maxEP             = 200
	mushroomsPerRound = 2
	epInterval        = 250 * time.Millisecond // 1 HP drained per tick -> 4 HP/s
)

// grantMushroom applies one mushroom buy: +epPerMushroom EP (cap maxEP), bumps the per-round count,
// and arms the regen clock so stepEP starts converting from `now`. The caller enforces the per-round
// limit before charging, so this always grants.
func (s *session) grantMushroom(now time.Time) {
	s.mushrooms++
	s.ep += epPerMushroom
	if s.ep > maxEP {
		s.ep = maxEP
	}
	s.epLast = now
}

// stepEP drains stored EP into HP on each match tick (run()-driven, like stepHeal): 1 HP per
// epInterval while EP remains, the player is alive, and HP is below max. Streams the new HP + EP as a
// PRI so both the health and energy bars move. Mirrors the reference (gated hp>0 && hp<max).
func (s *session) stepEP(now time.Time) {
	if s.ep == 0 {
		return
	}
	m := s.match
	hp := m.hp[s.entityID]
	// Only ACCRUE regen time while actively healing (alive, 0 < hp < maxHP). When regen is blocked — dead/
	// knocked, or already at FULL HP — anchor the clock to `now` and bail. Otherwise the elapsed time keeps
	// banking while the player sits at max HP, and the moment they drop below max (take a hit) `steps` is
	// huge and dumps a big chunk of EP into HP in one tick = the "full HP -> take damage -> instant heal" bug.
	if m.lifeOf(s.entityID) != lifeAlive || hp == 0 || hp >= m.settings.maxHP {
		s.epLast = now
		return
	}
	steps := int(now.Sub(s.epLast) / epInterval)
	if steps <= 0 {
		return
	}
	s.epLast = s.epLast.Add(time.Duration(steps) * epInterval)
	gain := uint16(steps)
	if gain > s.ep {
		gain = s.ep
	}
	if hp+gain > m.settings.maxHP {
		gain = m.settings.maxHP - hp
	}
	if gain == 0 {
		return
	}
	m.hp[s.entityID] = hp + gain
	s.ep -= gain
	s.sendVar(packet.CmdPRISync, s.match.priPayload(), 1) // stream the new HP + CUR_EP to everyone
}

// resetEP clears the per-round EP buffer + mushroom count (called at each round start).
func (s *session) resetEP() {
	s.ep = 0
	s.mushrooms = 0
}
