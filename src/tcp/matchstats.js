'use strict';

// Builds the tcp.MatchStatsRes the lobby shows after a match, from the settlement
// worker's per-player MatchResult data.
//
// RE (client COW::UIModelMatch::UnpackMatchResult): MatchStatsRes.match_stats and
// .income are NOT index-encoded arrays — they are nested SERIALIZED protos. The client
// UnSerializes MatchStatsRes.match_stats as proto.MatchStats (self stats + a
// `teammates` scoreboard) and MatchStatsRes.income as proto.MatchIncome (the rewards),
// then drives the post-match result UI. We push it as protocol STATS / MATCHSTATS_NTF.
const { lookup } = require('../protocol/protos');

// One scoreboard row (proto.TeammateStats) per player. `deads` = deaths, `score` is a
// simple kills-derived value until a real scoring model exists.
function teammateRow(pr, acct) {
  const a = acct || {};
  const si = a.selected_items || {};
  return {
    account_id: pr.account_id,
    nickname: a.nickname || '',
    kills: pr.kills | 0,
    deads: pr.deaths | 0,
    assists: 0,
    score: (pr.kills | 0) * 100,
    rank: pr.win ? 1 : 2,
    level: a.level || 1,
    avatar_id: si.avatar_id || 0,
    banner_id: si.banner_id || 0,
    head_pic: si.head_pic || 0,
    clan_name: (a.clan && a.clan.name) || ''
  };
}

// The shared scoreboard (one row per player), built once per match.
function buildTeammates(players, accts) {
  return players.map((pr) => teammateRow(pr, (accts || {})[pr.account_id]));
}

// The tcp.MatchStatsRes CONTENT bytes for ONE player: the nested proto.MatchStats
// (their stats + the whole scoreboard) and proto.MatchIncome (their rewards).
function buildStatsRes(pr, teammates, acct, matchId, playerCount) {
  const MatchStatsRes = lookup('MatchStatsRes');
  const MatchStats = lookup('MatchStats');
  const MatchIncome = lookup('MatchIncome');
  if (!MatchStatsRes || !MatchStats || !MatchIncome) return null;

  const a = acct || {};
  const si = a.selected_items || {};
  const level = a.level || 1;
  const coinsAfter = a.coins || 0;

  const matchStats = Buffer.from(MatchStats.encode(MatchStats.fromObject({
    account_id: pr.account_id,
    kills: pr.kills | 0,
    rank: pr.win ? 1 : 2,
    ranking_points: pr.rank_points | 0,
    match_mode: 6,
    player_count: playerCount,
    level,
    avatar_id: si.avatar_id || 0,
    banner_id: si.banner_id || 0,
    head_pic: si.head_pic || 0,
    teammates
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

module.exports = { buildTeammates, buildStatsRes };
