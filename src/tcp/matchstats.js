'use strict';

// Builds the tcp.MatchStatsRes the lobby shows after a match, from the settlement
// worker's per-player MatchResult data.
//
// RE (client COW::UIModelMatch::UnpackMatchResult + UIProfileCSMatchResultController):
// MatchStatsRes.match_stats / .income are NOT arrays — they are `bytes` holding NESTED
// SERIALIZED protos. match_stats -> proto.MatchStats, income -> proto.MatchIncome. The
// CS result UI reads TWO player lists from MatchStats: `teammates` (the LOCAL player's
// team) and `opponents` (the enemy team) — NOT one combined list — plus per-team scores
// in `teams_game_point` and the win/lose from `rank` (1 = win). Pushed as protocol
// STATS / MATCHSTATS_NTF.
const { lookup } = require('../protocol/protos');

// One proto.TeammateStats scoreboard row. `deads` = deaths; `score` is a simple
// kills+damage proxy until a real scoring model exists.
function teammateRow(pr, acct) {
  const a = acct || {};
  const si = a.selected_items || {};
  return {
    account_id: pr.account_id,
    nickname: a.nickname || '',
    kills: pr.kills | 0,
    deads: pr.deaths | 0,
    assists: 0,
    damage: pr.damage | 0,
    score: (pr.kills | 0) * 100 + (pr.damage | 0),
    rank: pr.win ? 1 : 2,
    level: a.level || 1,
    avatar_id: si.avatar_id || 0,
    banner_id: si.banner_id || 0,
    head_pic: si.head_pic || 0,
    clan_name: (a.clan && a.clan.name) || '',
    show_rank: true
  };
}

// The tcp.MatchStatsRes CONTENT bytes for ONE player (pr). Splits ALL players into
// `teammates` (same win outcome as pr = pr's team, self included) and `opponents` (the
// other team) — this is a team-based mode (CS), so a shared win/lose IS the team.
function buildStatsRes(pr, players, accts, matchId) {
  const MatchStatsRes = lookup('MatchStatsRes');
  const MatchStats = lookup('MatchStats');
  const MatchIncome = lookup('MatchIncome');
  if (!MatchStatsRes || !MatchStats || !MatchIncome) return null;

  const acctOf = (id) => (accts || {})[id] || {};
  const a = acctOf(pr.account_id);
  const si = a.selected_items || {};
  const level = a.level || 1;
  const coinsAfter = a.coins || 0;

  const sameTeam = (x) => (!!x.win) === (!!pr.win);
  const teammates = players.filter(sameTeam).map((x) => teammateRow(x, acctOf(x.account_id)));
  const opponents = players.filter((x) => !sameTeam(x)).map((x) => teammateRow(x, acctOf(x.account_id)));
  // teams_game_point[0] = LEFT grid (teammates) score, [1] = RIGHT (opponents) — the
  // client maps left=teammates / right=opponents, so this is viewer-relative kill totals.
  const teamKills = (pred) => players.filter(pred).reduce((s, x) => s + (x.kills | 0), 0);

  const matchStats = Buffer.from(MatchStats.encode(MatchStats.fromObject({
    account_id: pr.account_id,
    kills: pr.kills | 0,
    deaths: pr.deaths | 0,
    damage: pr.damage | 0,
    rank: pr.win ? 1 : 2,
    ranking_points: pr.rank_points | 0,
    match_mode: 6,
    player_count: players.length,
    real_player_count: players.length,
    level,
    avatar_id: si.avatar_id || 0,
    banner_id: si.banner_id || 0,
    head_pic: si.head_pic || 0,
    show_rank: true,
    teams_game_point: [teamKills(sameTeam), teamKills((x) => !sameTeam(x))],
    teammates,
    opponents
  })).finish());

  const income = Buffer.from(MatchIncome.encode(MatchIncome.fromObject({
    exp: pr.xp | 0,
    coins: pr.coins | 0,
    rank_points: pr.rank_points | 0,
    level_before: level,
    level_after: level,
    coins_before: Math.max(0, coinsAfter - (pr.coins | 0)),
    coins_after: coinsAfter
  })).finish());

  return Buffer.from(MatchStatsRes.encode(MatchStatsRes.fromObject({
    account_id: pr.account_id,
    match_id: Number(matchId) || 0, // uint64; synthetic (non-numeric) match ids -> 0
    level_before: level,
    level_after: level,
    income,
    match_stats: matchStats
  })).finish());
}

module.exports = { buildStatsRes };
