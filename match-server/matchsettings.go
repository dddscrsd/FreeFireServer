package main

import (
	"encoding/json"
	"fmt"
	"log"
)

// matchSettings is the per-match CS config. Defaults are the standard ruleset (the package consts /
// economy.go); a CUSTOM ROOM overrides them — the lobby decodes the host's room_setting/room_setting2
// and writes match:<id>:settings to Redis (see src/tcp/handlers/RoomStart.js), which we read at the
// first join. Absent / no-bus / unset field => the default is kept.
type matchSettings struct {
	maxRound    uint8    // CS match length (GRI MaxRound)
	roundsToWin uint8    // wins to end the match = (maxRound+1)/2
	baseCoins   []uint32 // per-round base income (economy.go csRoundBaseCoins by default)
	maxHP       uint16   // per-player max/full HP (player.go maxHP by default)
	noLoadout   bool     // NoLoadout flag: skip the free starter weapon (players buy everything)

	// unlimitedAmmo mirrors the room's UnlimitedAmmo flag (room_setting bit 0x2). The client
	// enforces infinite gun ammo itself (reload is a pure relay), but GLOO WALLS are server-tracked
	// and consumed via cmd 112 — so we must also skip the gloo deduction here, else the wall count
	// still drains despite "unlimited ammo". See gloo.go handlePlaceIcewall.
	unlimitedAmmo bool

	// speedScale / jumpScale are the room's move-speed / jump-height multipliers, PERCENT-encoded
	// (multiplier * 100: 1.0x=100, 0.5x=50, 2.0x=200) for PRI fields 50 (GAMEMODE_SPEEDSCALE) /
	// 52 (GAMEMODE_JUMPHEIGHTSCALE). The client divides by 100 (EFEDFMNEFNC @0x1fb6228) and applies
	// it to PlayerAttributes on BOTH the local and remote pawns (CS mode). 0 = OMIT the field (keep
	// the default 1.0x) — we must never send 0, which the client would read as a 0.0x freeze.
	speedScale uint16
	jumpScale  uint16

	// Raw custom-room bitfields, echoed verbatim into JoinMatchRes (cmd 100) so the CLIENT applies
	// its own client-side flags (UnlimitedAmmo / NoHud / NoAuxAim / NoSkill / FriendDmg / …) that the
	// server can't enforce. 0 => a normal (non-custom) match, i.e. vanilla rules.
	roomSetting  uint32 // ECustomRoomSetting  (GPKHLEAADHL)
	roomSetting2 uint32 // ECustomRoomSetting2 (OJIHIKIAFAI)
}

func defaultMatchSettings() matchSettings {
	return matchSettings{maxRound: maxRound, roundsToWin: roundsToWin, baseCoins: csRoundBaseCoins, maxHP: maxHP}
}

// wireSettings mirrors the JSON the lobby writes to match:<id>:settings.
type wireSettings struct {
	RoundCount   uint8           `json:"round_count"`
	MaxHP        uint16          `json:"max_hp"`
	Economy      []uint32        `json:"economy"`
	Flags        map[string]bool `json:"flags"`
	RoomSetting  uint32          `json:"room_setting"`  // raw ECustomRoomSetting  bitfield (echoed to the client)
	RoomSetting2 uint32          `json:"room_setting2"` // raw ECustomRoomSetting2 bitfield
	SpeedMul     float64         `json:"speed_mul"`     // move-speed multiplier (1.0/0.5/1.25/2.0); 0 = default
	JumpMul      float64         `json:"jump_mul"`      // jump-height multiplier (1.0/2.0/4.0); 0 = default
}

// loadMatchSettings seeds m.settings with the defaults, then (if the bus is up and this is a real
// match id) reads + applies the custom-room overrides for match `mid`. Best-effort: any error or a
// missing key leaves the defaults in place. Called once at the first join, before the round setup
// reads maxRound.
func (m *Match) loadMatchSettings(mid uint64) {
	m.settings = defaultMatchSettings()
	if eventBus == nil || mid == 0 {
		return
	}
	raw, err := eventBus.Get(fmt.Sprintf("match:%d:settings", mid))
	if err != nil || raw == "" {
		return
	}
	var w wireSettings
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		log.Printf("[mm-udp] match %d: bad settings json: %v", mid, err)
		return
	}
	if w.RoundCount > 0 {
		m.settings.maxRound = w.RoundCount
		m.settings.roundsToWin = (w.RoundCount + 1) / 2
	}
	if len(w.Economy) > 0 {
		m.settings.baseCoins = w.Economy
	}
	if w.MaxHP > 0 {
		m.settings.maxHP = w.MaxHP
	}
	if w.Flags["noLoadout"] {
		m.settings.noLoadout = true
	}
	if w.Flags["unlimitedAmmo"] {
		m.settings.unlimitedAmmo = true
	}
	m.settings.roomSetting = w.RoomSetting   // echoed verbatim to the client in JoinMatchRes (cmd 100)
	m.settings.roomSetting2 = w.RoomSetting2 // ""
	// Percent-encode the speed/jump multipliers for PRI 50/52; omit an effective 1.0x (== 100) so a
	// default-speed room keeps the client's own scale instead of being pinned. round() via +0.5.
	if w.SpeedMul > 0 {
		if v := uint16(w.SpeedMul*100 + 0.5); v != 100 {
			m.settings.speedScale = v
		}
	}
	if w.JumpMul > 0 {
		if v := uint16(w.JumpMul*100 + 0.5); v != 100 {
			m.settings.jumpScale = v
		}
	}
	log.Printf("[mm-udp] match %d custom-room settings: maxRound=%d roundsToWin=%d maxHP=%d noLoadout=%t unlimitedAmmo=%t speed=%d jump=%d roomSetting=0x%x/0x%x economy=%v",
		mid, m.settings.maxRound, m.settings.roundsToWin, m.settings.maxHP, m.settings.noLoadout, m.settings.unlimitedAmmo, m.settings.speedScale, m.settings.jumpScale, m.settings.roomSetting, m.settings.roomSetting2, m.settings.baseCoins)
}
