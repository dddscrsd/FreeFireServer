'use strict';

/**
 * Static LoginDescRes payload for the LoginGetDesc handshake endpoint.
 *
 * This is the editable, trimmed replacement for the old pre-serialized
 * protocol/LoginDescRes.bin (built from original_to_read/gen_logindescres.py).
 * Every repeated list from that blob has been reduced to ONE representative
 * entry so the full field structure is present but bloat-free — add/remove
 * entries here to control adverts, activities, gacha banners, store tabs, etc.
 *
 * Field names are snake_case to match the .proto (protobufjs is loaded with
 * keepCase: true); the router encodes this object against the LoginDescRes type.
 *
 * NOTE: switchs_res.switchs was trimmed from 199 entries to 1. Unlike the other
 * lists these are feature TOGGLES keyed by id (config, not events); if the lobby
 * hides/disables something it should show, re-populate the needed switches here.
 */
module.exports = {
    "advert_res": {
      "advert_items": [
        {
          "id": 20190712,
          "type": 25,
          "sort_id": 2,
          "language": "default",
          "advertisment_url": "OB17/Installment/Tutorial2BR_pt.png",
          "ad_start_time": 1553050800,
          "ad_end_time": 1868929199
        }
      ]
    },
    "switchs_res": {
      "switchs": [
        {
          "is_open": true
        }
      ]
    },
    "activity_res": {
      "activity_descs": [
        {
          "group_id": 2511244,
          "activity_id": 25112443,
          "act_title": "Defenda sua posição!!",
          "act_text": "Use Paredes de Gel pra surfar na onda do oniguiri!",
          "award_context": "Use 0/15 Paredes de Gel",
          "sort_id": 26,
          "start_time": 1732806000,
          "end_time": 1733122799,
          "cdt_value": 15,
          "awards": [
            {
              "award_id": 907104645,
              "award_num": 1
            }
          ],
          "table_type": 2,
          "original_start_time": "2024-11-28 12:00:00",
          "original_end_time": "2024-12-02 03:59:59"
        }
      ],
      "activity_festivals": [
        {
          "event_id": 189,
          "table_type": 3,
          "language": "default",
          "event_title": "Blue Lock",
          "start_time": "2024-11-20 04:00:00",
          "end_time": "2024-12-09 03:59:59"
        }
      ],
      "entrance_list": [
        {
          "id": 280,
          "show_time": 1605618000,
          "start_time_stamp": 1605618000,
          "end_time_stamp": 1605855599,
          "cdn_url": "https://dl.cdn.freefiremobile.com/common/test/PreviewBG.jpg",
          "start_time": "2020-11-17 10:00:00",
          "end_time": "2020-11-20 03:59:59"
        }
      ],
      "bingo_info": [
        {
          "level": 1,
          "key_id": 801000079,
          "init_key_num": 1,
          "cost_key_num": 1,
          "cost_diamond": 10,
          "start_time": 1553238000,
          "end_time": 1554965999
        }
      ],
      "bingo_rewards_info": [
        {
          "id": 1002,
          "level": 2,
          "activity_ids": [
            400000010
          ],
          "awards": [
            {
              "award_id": 802000001,
              "award_num": 1
            }
          ]
        }
      ],
      "lobby_game_enter_styles": [
        {
          "event_id": 2,
          "start_time": "2024-10-23 10:00:00",
          "end_time": "2024-12-04 19:59:59",
          "lobby_start_icon": "UI_Lobby_Btn_EnterGame_01",
          "lobby_start_vfx": "VFX_Booyahday24_Sys_Start"
        }
      ]
    },
    "gacha_res": {
      "gacha_desc_list": [
        {
          "chest_id": 88,
          "chest_type": {
            "chest_id": 88,
            "priority": 912,
            "coin_type": 2,
            "start_time": "2024-11-29 00:00:00",
            "end_time": "2024-12-08 23:59:59",
            "chest_name": "T_38_LXW_GACHA_UNLIMITED_TITLE",
            "exchange_itemid": [
              0
            ],
            "start_time_stamp": 1732849200,
            "end_time_stamp": 1733713199,
            "once_price": 9,
            "ten_price": 79,
            "color_id": 2,
            "once_num": 1,
            "ten_num": 10,
            "show_model_male": 102000019,
            "show_model_female": 101000018,
            "chest_sub_id": 4614,
            "drop_probability_switch": true,
            "open_priority_switch": 715,
            "forge_tab_id": 884614,
            "show_type": [
              0
            ],
            "multi_once_price": [
              0
            ],
            "price_one_type": [
              0
            ],
            "independent_entrance_type": 1
          },
          "item_list_with_jackpot": [
            {
              "items": [
                {
                  "item_id": 710046033,
                  "repeated_item_id": 801046101,
                  "item_num": 1,
                  "reward_level": 1,
                  "drop_up_ratio": 2,
                  "is_drop_up_buffed": true,
                  "id": 1
                }
              ],
              "jackpot": 884614
            }
          ]
        }
      ]
    },
    "ranking_res": {
      "season_info": {
        "id": 7,
        "open_time": 1730451600,
        "end_time": 1735720199,
        "season_name": "S7",
        "starting_ranking_points": 1000,
        "map_id": "1",
        "season_award_id": 907104625,
        "season_award_rank": 207,
        "season_peak_award_id": 203046030,
        "season_peak_award_rank": 219,
        "heroic_mark_switch": true
      },
      "awards": [
        {
          "rank": 201
        }
      ]
    },
    "hide_res": {
      "avatar_id": [
        102000023
      ],
      "ip_expired_avatar_id": [
        101000023
      ]
    },
    "treasure_res": {
      "boxes": [
        {
          "id": 400046065,
          "treasure_boxes": [
            {
              "award_id": 1307000001,
              "is_preview": true,
              "is_big_reward": true,
              "award_num": 2
            }
          ]
        }
      ],
      "crates": [
        {
          "crate_id": 400026142,
          "treasure_group": [
            {
              "group_id": 400026116,
              "num_group": 1,
              "icon": "FF_Mvp_Icon04"
            }
          ]
        }
      ]
    },
    "bundle_res": {
      "bundle_show": [
        {
          "id": 710046061,
          "bundles": [
            {
              "award_id": 921046023,
              "is_preview": true,
              "award_num": 1,
              "return_id": 800000301,
              "return_num": 4,
              "award_time": 5184000
            }
          ]
        }
      ]
    },
    "gift_store_res": {
      "store_id": 14,
      "open_time": 1551150000,
      "close_time": 1866769199,
      "giver_level": 15,
      "receiver_level": 10,
      "gift_time_limited": 3,
      "gift_num_limited": 5
    },
    "ranking_match_param": {
      "solo_chicken_points": 5,
      "dual_chicken_points": 5,
      "squad_chicken_points": 5,
      "knockdown_points": 2,
      "revive_points": 1,
      "solo_battle_score_limit": 13,
      "solo_kill_add": 2,
      "group_battle_score_limit": 13,
      "group_kill_add": 1,
      "damage_per_score": 200,
      "down_points": -1,
      "first_win_rank": 50,
      "first_win_kill": 1,
      "ranking_token_id": 800000303,
      "most_token_given": 4000,
      "ranking_extra_coins": 1.2000000476837158,
      "ranking_extra_exp": 1.2000000476837158,
      "birth_island_quit_deduct": -20,
      "rp_range_add_bp_lower": 1400,
      "rp_range_add_bp_upper": 99999,
      "relife_teammate_points": 1,
      "min_rank_show": 2
    },
    "fullscreen_cg_res": {
      "fullscreen_cgs": [
        {
          "id": 68,
          "anim_id": 168,
          "anim_type": 4,
          "system_pos": 15,
          "go_pos": 994401,
          "skin_resource_list": [
            "VD_PREVIEWEXECUTION_PERSONA_KATANA"
          ]
        }
      ]
    },
    "championship_basic_info_res": {
      "open_infos": [
        {
          "championship_type": 1003,
          "championship_id": 1,
          "season_start_time": 1699459200,
          "season_end_time": 1699876800,
          "trial_start_time": 1699459200,
          "trial_end_time": 1699840800,
          "final_start_time": 1699840799,
          "final_end_time": 1699840800,
          "map_id": "1;3;22;14;4",
          "entrance_open_time": 1699459200,
          "entrance_end_time": 1699876800
        }
      ],
      "limited_level": 12,
      "limited_rank": 1550,
      "reward_infos": [
        {
          "championship_type": 1003,
          "championship_id": 1,
          "lower_bound": 1,
          "upper_bound": 50,
          "end_award": [
            {
              "award_id": 907103823,
              "award_num": 1
            }
          ]
        }
      ],
      "setting_infos": [
        {
          "championship_type": 1003,
          "championship_id": 1,
          "trial_match_name": "T_42_LYF_FFC_SOLO_NAME",
          "trial_match_num": 25,
          "trial_match_choose_num": 12,
          "final_match_choose_num": 12,
          "min_match_num": 12,
          "limited_item_type": 1,
          "limited_item_id": 803000002,
          "limited_item_num": 1,
          "seniority_list": 3000,
          "clan_icon_id": "UI_Profile_Icon_Score02",
          "is_animation_open": true,
          "login_animation_resource": "https://dl.dir.freefiremobile.com/common/web_event/hash/d14554fbfd5936a85812311ec2893cd5.png",
          "mode_name": "T_42_LYF_FFC_SOLO_NAME",
          "is_notice_open": true,
          "notice_cdn1": "https://dl.dir.freefiremobile.com/common/OB42/FFC/BR1_pt.jpg",
          "notice_cdn2": "https://dl.dir.freefiremobile.com/common/OB42/FFC/BR2_pt.jpg",
          "notice_cdn3": "https://dl.dir.freefiremobile.com/common/OB42/FFC/1000x620-Informative-Banner3_pt.jpg",
          "team_scale_type": 1,
          "game_mode": 1,
          "disable_weapon_skin": true,
          "leaderboard_local_size": 3000,
          "language": "default",
          "background_resource": "https://dl.dir.freefiremobile.com/common/web_event/hash/f0b0cc41f5aae4d13981e08f756e10bb.jpg"
        }
      ],
      "score_bases": [
        {
          "championship_type": 1003,
          "first_ranking_point": 100,
          "damage_per_get": 100,
          "alive_per_get": 300,
          "ranking_extra_coins": 1.2000000476837158,
          "ranking_extra_exps": 1.2000000476837158,
          "birth_island_quit_deduct": -3,
          "kill_points": 1,
          "game_mode": 1
        }
      ],
      "team_settings": [
        {
          "player_num_limit": 1,
          "limit_num": 30,
          "clean_time": 7,
          "player_apply_num": 30,
          "team_invite_num": 60,
          "team_exit_cd": 600,
          "team_join_cd": 600,
          "team_scale_type": 1,
          "player_num_min": 1
        }
      ]
    },
    "store_tab_res": {
      "store_tables": [
        {
          "table_type": 3,
          "language": "default",
          "table_name": "T_16_W_LEGENDBOX",
          "mall_type": 2,
          "sort_id": 3
        }
      ]
    },
    "fullscreen_item_res": {
      "fullscreen_items": [
        {
          "item_id": 808000001
        }
      ]
    },
    "avatar_awaken_res": {
      "infos": [
        {
          "awaken_avatar_id": 101000015,
          "original_avatar_id": 101000006,
          "unlock_time": "2020-06-30 20:00:00",
          "awards": [
            {
              "award_id": 101000015,
              "award_num": 1
            }
          ],
          "awaken_description": "T_27_A_AWAKEN_K_CONTENT",
          "awaken_title": "T_27_A_AWAKEN_K_TITLE"
        }
      ]
    },
    "diamond_cost_res": {
      "diamond_cost": 800
    },
    "cs_ranking_res": {
      "season_info": {
        "id": 1,
        "open_time": 1727773200,
        "end_time": 1733041799,
        "season_name": "Season 1",
        "map_id": "1",
        "season_award_id": 907104624,
        "season_award_rank": 209,
        "season_peak_award_id": 203046029,
        "season_peak_award_rank": 219,
        "heroic_mark_switch": true
      },
      "awards": [
        {
          "rank": 201
        }
      ]
    },
    "gopos_res": {
      "go_pos": [
        {
          "id": 710036001,
          "go_pos": "103",
          "sub_pos": "710036001"
        }
      ]
    },
    "exchange_currency_all_res": {
      "exchange_currency_desc": [
        {
          "id": 801001834
        }
      ]
    },
    "pay_level_config_res": {
      "pay_level_config": [
        {
          "pool": {
            "id": 1,
            "unique_id": 10000,
            "start_time": 1598065200,
            "end_time": 1885604399
          },
          "rewards": [
            {
              "id": 1,
              "level": 1,
              "awards": [
                {
                  "award_num": 300
                }
              ]
            }
          ]
        }
      ]
    },
    "role_debris_desc": {
      "role_debris_tables": [
        {
          "debris_id": 800000050,
          "purchase_count": 10,
          "gold_price": 100,
          "diamond_price": 2
        }
      ]
    },
    "opposite_sex_model_res": {
      "opposite_sex_models": [
        {
          "male_model_id": 102200019,
          "female_model_id": 101100018,
          "start_timestamp": 1526572800,
          "end_timestamp": 1782662399
        }
      ]
    },
    "weapon_skin_system_time": 1603508400,
    "rate_app_switch_res": {
      "switch_desc": {
        "region": "BR",
        "ios_open": true,
        "gp_open": true,
        "game_mode1": 1,
        "rank1": 3,
        "game_mode2": 15,
        "rank2": 1,
        "cd": 30
      }
    },
    "live_desc_res": {
      "descs": [
        {
          "id": 1,
          "tab_name": "T_26_Z_LIVE_TITLE_ESPORTS",
          "web_link": "https://esportslive.us.freefiremobile.com/?access_token=<GARENA_TOKEN>&lang=<FF_LANGUAGE>&region=<FF_REGION>"
        }
      ]
    },
    "android_apps_to_detect_res": {
      "android_apps_to_detect_res": [
        {
          "id": 1,
          "bundle_identifier": "com.tencent.ig"
        }
      ]
    },
    "optional_bundle_res": {
      "optional_bundle_show": [
        {
          "id": 170042514,
          "bundles": [
            {
              "option_order": 1,
              "award_id": 801039059,
              "award_num": 50,
              "is_preview": true
            }
          ]
        }
      ]
    },
    "exchange_currency_local": {
      "exchange_currency_local_desc": [
        {
          "id": 800000303,
          "origin": "T_29_AG_BR_RANKED",
          "start_time": "2023-02-01 00:00:00",
          "end_time": "2033-02-01 00:00:00",
          "go_pos": 22,
          "sub_go_pos": "V2_matchMode_5"
        }
      ]
    },
    "universal_link_setting_res": {
      "universal_link_settings": [
        {
          "universal_link": "https://game.ff.garena.com/index.html",
          "deep_link": "freefire://process?action=mail",
          "start_time": 1641524400,
          "end_time": 1646708400
        }
      ]
    },
    "share_setting_desc": {
      "share_settings": [
        {
          "share_id": 1,
          "share_link_fb": "https://ffshare.garena.com/?region=<FF_REGION>&lang=<FF_LANGUAGE>",
          "share_link_vk": "https://ffshare.garena.com/?region=<FF_REGION>&lang=<FF_LANGUAGE>",
          "share_link_line": "https://ffshare.garena.com/?region=<FF_REGION>&lang=<FF_LANGUAGE>",
          "share_title_key": "TXT_SHARE_MATCH_RESULT_FB_NAME",
          "share_content_key": "TXT_SHARE_MATCH_RESULT_FB_DESC",
          "share_caption_key": "TXT_SHARE_MATCH_RESULT_FB_CAPTION",
          "sys_text_key_ios": "TXT_SHARE_MATCH_RESULT_IOS_CAPTION",
          "sys_text_key_android": "TXT_SHARE_MATCH_RESULT_ANDROID_CAPTION"
        }
      ]
    },
    "friend_desc": {
      "relation_type_info_desc": [
        {
          "relation_item_id": 1600000001,
          "reject_countdown": 28800,
          "relation_break_up_cd": 86400,
          "item_intimacy_diamond_rate": 0.10000000149011612,
          "item_intimacy_coin_rate": 0.0010000000474974513,
          "intimacy_week_max": 100,
          "relation_friend_max": 1
        }
      ]
    },
    "skin_timeline_res": {
      "desc": [
        {
          "id": 1,
          "type": 1,
          "res_conf1": "913046004",
          "start_time": 1729170000,
          "end_time": 1733317199
        }
      ]
    },
    "quit_control_desc": {
      "quit_control_settings": [
        {
          "match_mode": 3,
          "game_mode": 45
        }
      ]
    }
};
