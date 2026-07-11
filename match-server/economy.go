package main

// CS round economy — the REAL Free Fire Clash Squad model, RE'd from the client's custom-room
// "Advanced Settings -> Economy" defaults (protocol/per_round_money.png + per_event_money.png).
// Coins are a RUNNING BALANCE: purchases subtract during the buy phase, and this grant is added at
// each round end. This replaces the old flat 500*(round+1) + 500*kills + 500*win.
//
// These are the DEFAULT values (also what a non-custom CS match uses). Advanced custom rooms can
// override the whole table + event bonuses via cs_advanced_setting (a later phase); until then a
// per-Match override just swaps these numbers.

// csRoundBaseCoins: table[0] is the STARTING money (seeded before round 1); table[i>=1] is the base
// income EARNED at the END of round i. So a player starts round 1 with 500, then earns 900 at the
// end of round 1, 1100 at the end of round 2, ... Rounds past the table earn the LAST value — i.e.
// round 7 onward is fixed at 3000 (confirmed with the user).
var csRoundBaseCoins = []uint32{500, 900, 1100, 1700, 2100, 2400, 3000}

const (
	csWinRoundBonus   uint32 = 500  // event: won the round
	csLossRoundBonus  uint32 = 200  // event: lost the round (base; the streak bonus stacks on top)
	csKillBonus       uint32 = 200  // event: per kill this round
	csLossStreak2     uint32 = 900  // event: 2 consecutive round losses
	csLossStreak3     uint32 = 1500 // event: 3+ consecutive round losses
	csFirstBloodBonus uint32 = 100  // event: got the round's first kill (tracked via Match.firstBlood, awarded in endFight)
)

// csStartingCoins is the money a player begins the match with = the economy table's FIRST entry.
func csStartingCoins(table []uint32) uint32 {
	if len(table) == 0 {
		table = csRoundBaseCoins
	}
	return table[0]
}

// csRoundIncome is the base coins EARNED at the END of a 1-based round from `table` (a custom room's
// economy preset, or csRoundBaseCoins when nil/empty). Since table[0] is the STARTING money, round R
// earns table[R] (round 1 -> table[1] = 900), holding the last table value for rounds beyond it
// (round 7+ => 3000 with the default table).
func csRoundIncome(round int, table []uint32) uint32 {
	if len(table) == 0 {
		table = csRoundBaseCoins
	}
	if round < 1 {
		round = 1
	}
	if round >= len(table) {
		return table[len(table)-1]
	}
	return table[round]
}

// csEvents are the per-match CS economy EVENT bonuses. The defaults (defaultCSEvents) are the consts
// above — the same values a non-custom CS match uses; an ADVANCED custom room can override them
// (once RoomCreateCSEco.csv is captured, see src/tcp/csadvanced.js). Held on matchSettings.events.
type csEvents struct {
	winRound    uint32 // won the round
	perKill     uint32 // per kill this round
	lossRound   uint32 // lost the round (base; the streak bonus stacks on top)
	lossStreak2 uint32 // 2 consecutive round losses
	lossStreak3 uint32 // 3+ consecutive round losses
	firstBlood  uint32 // got the round's first cross-team kill (tracked via Match.firstBlood, awarded in endFight)
}

// defaultCSEvents is the standard ruleset (also what a non-custom CS match uses).
func defaultCSEvents() csEvents {
	return csEvents{
		winRound: csWinRoundBonus, perKill: csKillBonus, lossRound: csLossRoundBonus,
		lossStreak2: csLossStreak2, lossStreak3: csLossStreak3, firstBlood: csFirstBloodBonus,
	}
}

// lossBonus is the loss-side round bonus for a team that just lost, given its consecutive-loss
// streak (INCLUDING this round). The base loss bonus always applies; a 2- or 3-round losing streak
// stacks its event bonus on top (the highest reached rung). NOTE: whether the streak REPLACES or
// STACKS on the base loss bonus wasn't recoverable from the obfuscated client; we own the match
// server, so this is our (stack, additive) choice per the user's "it adds the per-event amounts" —
// tune here if a live buy-phase balance says otherwise.
func (e csEvents) lossBonus(streak uint8) uint32 {
	b := e.lossRound
	switch {
	case streak >= 3:
		b += e.lossStreak3
	case streak == 2:
		b += e.lossStreak2
	}
	return b
}
