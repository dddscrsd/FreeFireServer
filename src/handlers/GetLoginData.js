/**
 * GetLoginData  (LoginReq -> proto.LoginRes)  [Bearer auth]
 *
 * Ported verbatim from login.js (handleGetLoginData).
 *
 * Returns the full account snapshot the client needs to enter the lobby.
 * Resolved 1.70 response type: proto.LoginRes (the message in ServiceMessage-
 * TypeHTTP carrying account_id/nickname/level/region/clan_id/coins/gems/
 * notification_channel/rank_info/elite_pass_basic_info — see
 * protocol/protos/proto.proto:12169). (reference: get_login_data @ htpp.py:2295,
 * AccountResponseEncoder.)
 */

'use strict';

const config = require('../../config/default');
const { getLocalIp } = require('../utils/address');
const { requireAccount, nowSecs, serverUrl, DEFAULT_REGION } = require('./_shared');

async function handleGetLoginData(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {}; // router already guards, but stay defensive.

  const localIp = await getLocalIp();
  const region = account.region || DEFAULT_REGION;

  // Realtime endpoints the client connects to after login (LoginRes carries
  // them). Host comes from config, else the request host, else localhost.
  const p = (config.protocol) || {};
  const host =
    localIp ||
    ((ctx.req.headers.host || '').split(':')[0]) ||
    '127.0.0.1';
  const gameServerIp = `${host}:${p.gameServerPort || '10100'}`;
  const chatAddr = `${host}:${p.chatPort || '10200'}`;
  // TCP gateway the client opens for server push — single-sourced from ports.tcp.
  const notificationChannel = `${host}:${(config.ports && config.ports.tcp) || p.notificationPort || '10300'}`;

  ctx.logger.info(`[login] GetLoginData uid=${account.uid} region="${region}" game=${gameServerIp}`);

  return {
    account_id: account.uid,
    account_type: account.account_type || 0,
    region: region,
    nickname: account.nickname || 'Player',
    create_at: account.created_at ? Math.floor(account.created_at / 1000) : nowSecs(),
    level: account.level || 1,
    exp: account.exp || 0,
    chat_server: 1,
    coins: account.coins || 0,
    gems: account.gems || 0,
    notification_channel: notificationChannel,
    voice_server: 1,
    voice_type: 1,
    event_log_url: serverUrl(ctx) + '/',
    region_id_mapping: [{ id: 1, region }],
    clan_id: (account.clan && account.clan.id) || 0,
    server_time: nowSecs(),
    noti_region: region,
    role: account.role || 1,
    is_emulator: false,
    // queue gate: ProcessLastLoginRes treats a non-zero need_waiting_secs as
    // "in login queue" -> shows a queue popup and re-requests login every 5s.
    // Both MUST be 0 to enter the lobby.
    queue_position: 0,
    need_waiting_secs: 0,
    has_elite_pass: !!account.elite_pass,
    badge_id: account.ep_badge_id || 1001000021,
    badge_cnt: account.ep_badge_count || 999,
    chat_addr: chatAddr,
    // 1.70 replaces the old single game_server_id with a node list; the client
    // connects to the realtime game server using this.
    game_server_node_manager: [
      { region, node_name: 'node1', server_ip: gameServerIp, is_default: true, warn_capacity: 1000 }
    ],
    show_rank: true,
    elite_pass_basic_info: {
      owned_pass: !!account.elite_pass,
      ep_event_id: account.ep_event_id || 0,
      start_time: account.ep_start_time || 0,
      end_time: account.ep_end_time || 0,
      ep_badge: account.ep_badge_id || 0,
      gold_limit_improved: 0,
      owned_fp_challenge: false,
      badge_cnt: account.ep_badge_count || 0
    },
    rank_info: {
      account_id: account.uid,
      season_id: 7,
      rank: account.rank || 1,
      max_rank: account.rank || 1,
      ranking_points: account.ranking_points || 0,
      show_rank: true,
      // last_season_info intentionally OMITTED — see GetPlayerRankingInfo: a
      // prior season triggers the past-season popup whose particle animation
      // crashes the 1.70 client. Fresh accounts have no completed prior season.
      match_token_num: 0,
      ranking_bot_points: 0,
      peak_rank_pos: 0
    },
    return_at: 0,
    network_log_url: serverUrl(ctx) + '/',
    ping_addr_desc_list: [
      {ip: host, is_traceroute: false}
    ],
    ip_region: "https://brnetwork.ggblueshark.com/",
    ranking_peak_threshold: 0,
    last_login_at: account.token_created_at || 0,
    is_anti_addiction_open: false,
    // --- remaining LoginRes fields ---------------------------------------
    // NOTE: proto3 does not put default scalars (0 / "" / false) on the wire,
    // so the zero-valued ones below won't appear in a defaults:false decode —
    // the client reads them as their defaults regardless. They are listed for
    // completeness; message/enum fields ARE emitted so the client sees them.

    age_state: 2,                       // tcp.AgeState ADULT (avoids under-age limits)
    ab_test_choices: { choices: [] },
    mem_value: '',
    register_variant: 0,
    weapon_rack: { weapon_ids: (account.weapon_rack && account.weapon_rack.weapon_ids) || [] },
    vehicle_display: 0,
    gloo_display: 0,
    clan_channel_secret: (account.clan && account.clan.channel_secret) || '',
    check_name: [],
    skyboard_display: 0,
    mic_muted_seconds: 0,
    is_optional_item_ab_test: false
    // blacklist (field 49) intentionally OMITTED: a non-null blacklist makes the
    // client (ProcessLastLoginRes) treat the account as forbidden.
  };
}

module.exports = {
  endpoint: 'GetLoginData',
  reqType: 'LoginReq',
  resType: 'LoginRes',
  handler: handleGetLoginData
};
