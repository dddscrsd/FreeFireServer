package main

// CS round economy — the REAL Free Fire Clash Squad model, RE'd from the client's custom-room
// "Advanced Settings -> Economy" defaults (protocol/per_round_money.png + per_event_money.png).
// Coins are a RUNNING BALANCE: purchases subtract during the buy phase, and this grant is added at
// each round end. This replaces the old flat 500*(round+1) + 500*kills + 500*win.
//
// These are the DEFAULT values (also what a non-custom CS match uses). Advanced custom rooms can
// override the whole table + event bonuses via cs_advanced_setting (a later phase); until then a
// per-Match override just swaps these numbers.

// csRoundBaseCoins is the per-round BASE income (index 0 = round 1). Rounds past the table award the
// LAST value — i.e. round 7 onward is fixed at 3000 (confirmed with the user).
var csRoundBaseCoins = []uint32{500, 900, 1100, 1700, 2100, 2400, 3000}

const (
	csWinRoundBonus   uint32 = 500  // event: won the round
	csLossRoundBonus  uint32 = 200  // event: lost the round (base; the streak bonus stacks on top)
	csKillBonus       uint32 = 200  // event: per kill this round
	csLossStreak2     uint32 = 900  // event: 2 consecutive round losses
	csLossStreak3     uint32 = 1500 // event: 3+ consecutive round losses
	csFirstBloodBonus uint32 = 100  // event: got the round's first kill (TODO: not tracked yet)
)

// csBaseCoins returns the base income for a 1-based round from `table` (a custom room's economy
// preset, or csRoundBaseCoins when nil/empty), holding the last table value for rounds beyond it
// (e.g. round 7+ => 3000 with the default table).
func csBaseCoins(round int, table []uint32) uint32 {
	if len(table) == 0 {
		table = csRoundBaseCoins
	}
	if round < 1 {
		round = 1
	}
	if round > len(table) {
		round = len(table)
	}
	return table[round-1]
}

// csLossBonus is the loss-side round bonus for a team that just lost, given its consecutive-loss
// streak (INCLUDING this round). The base loss bonus always applies; a 2- or 3-round losing streak
// stacks its event bonus on top (the highest reached rung). NOTE: whether the streak REPLACES or
// STACKS on the base loss bonus wasn't recoverable from the obfuscated client; we own the match
// server, so this is our (stack, additive) choice per the user's "it adds the per-event amounts" —
// tune here if a live buy-phase balance says otherwise.
func csLossBonus(streak uint8) uint32 {
	b := csLossRoundBonus
	switch {
	case streak >= 3:
		b += csLossStreak3
	case streak == 2:
		b += csLossStreak2
	}
	return b
}
