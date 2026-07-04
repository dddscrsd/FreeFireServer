'use strict';

/**
 * MATCHMAKING / GAMEOPENINGINFO  (GameOpeningInfoReq -> GameOpeningInfoRes)
 *
 * The client asks which game modes are currently open (right after connecting,
 * and when opening the mode-select screen). We advertise a single mode:
 *   map_id 1 (Paradise) + game_mode 15 (CS / ContraSquad)  -> config row 1015.
 * (see protocol/gamemode_and_map_config.txt)
 *
 * game_mode_name_list / mode_level_limit_list / ranking_level_limit_list are
 * optional and omitted; the client does not require them.
 */

const { EProtocol, EMatchmaking } = require('../protocol');

// Currently open game modes. Add entries here to open more.
const OPEN_MODES = [
  { 
    map_id: 1, 
    game_mode: 15,
    match_mode: 1, 
    sort_id: 1, 
    start_time: "00:00",
    end_time: "23:59",
    is_new: true,
    is_live_open: true,
    config_start_time: "2020-01-01 00:00:00",
    config_end_time: "2030-12-31 23:59:59",
    weekday: "1;2;3;4;5;6;7",
    tips: "Permanent",
    language: "en",
    limited_count: 0,
    tag: 1,
    difficulty: "1",
    visual_map: "https://foices.github.io/minhas_resources/contra_squad.png"
  }
];

function handler(reqObj, ctx) {
  ctx.logger.info(
    `[tcp] GameOpeningInfo region="${reqObj.region || ''}" lang="${reqObj.language || ''}" ` +
    `-> ${OPEN_MODES.length} open mode(s)`
  );
  return {
    opening_info_list: { gameOpeningInfos: OPEN_MODES },
    // server timezone offset (east of UTC) in seconds; only relevant to timed
    // events, of which we advertise none.
    timezone_offset_secs: -new Date().getTimezoneOffset() * 60
  };
}

module.exports = {
  protocol: EProtocol.MATCHMAKING,        // 3
  subcmd: EMatchmaking.GAMEOPENINGINFO,   // 7
  reqType: 'GameOpeningInfoReq',
  resType: 'GameOpeningInfoRes',
  handler
};
