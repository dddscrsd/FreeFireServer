package main

import (
	"encoding/json"
	"fmt"
	"log"
)

// matchSettings is the per-match CS config. Defaults are the standard ruleset (the package consts /
// economy.go); a CUSTOM ROOM overrides them — the lobby decodes the host's room_setting/room_setting2
// and writes match:<id>:settings to Redis (see src/tcp/handlers/RoomStart.js), which we read at the
// first join. Absent / no-bus / unset field => the default is kept. (maxHP + gameplay flags: TODO.)
type matchSettings struct {
	maxRound    uint8    // CS match length (GRI MaxRound)
	roundsToWin uint8    // wins to end the match = (maxRound+1)/2
	baseCoins   []uint32 // per-round base income (economy.go csRoundBaseCoins by default)
}

func defaultMatchSettings() matchSettings {
	return matchSettings{maxRound: maxRound, roundsToWin: roundsToWin, baseCoins: csRoundBaseCoins}
}

// wireSettings mirrors the JSON the lobby writes to match:<id>:settings.
type wireSettings struct {
	RoundCount uint8           `json:"round_count"`
	MaxHP      uint16          `json:"max_hp"`
	Economy    []uint32        `json:"economy"`
	Flags      map[string]bool `json:"flags"`
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
	log.Printf("[mm-udp] match %d custom-room settings: maxRound=%d roundsToWin=%d economy=%v",
		mid, m.settings.maxRound, m.settings.roundsToWin, m.settings.baseCoins)
}
