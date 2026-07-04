/**
 * GetAttendanceList  (-> CSGetAttendanceListRes)
 *
 * Ported verbatim from ported_8.js (handleGetAttendanceList).
 * reference: py 1972 — builds a 7-day attendance reward table. Mapped onto our
 * proto shape (AttendanceList -> AttendanceItem -> AwardDesc).
 */

'use strict';

const { requireAccount } = require('./_shared');

const REF_ATTENDANCE = [
  { day: 1, reward_id: 10000004, reward_amount: 500 },
  { day: 2, reward_id: 102, reward_amount: 10 },
  { day: 3, reward_id: 103, reward_amount: 1 },
  { day: 4, reward_id: 104, reward_amount: 1 },
  { day: 5, reward_id: 105, reward_amount: 1 },
  { day: 6, reward_id: 106, reward_amount: 1000 },
  { day: 7, reward_id: 107, reward_amount: 25 }
];

function handleGetAttendanceList(reqObj, ctx) {
  const account = requireAccount(ctx);
  if (!account) return {};
  const attendance = REF_ATTENDANCE.map((r) => ({
    id: r.day,
    signed: 0,
    awards: [{ award_id: r.reward_id, award_num: r.reward_amount }],
    is_big_prize: r.day === 7
  }));
  return {
    attendance_list: [
      {
        attendance,
        end_time: 1762536000,
        url: '',
        loc_key: 'Attendance',
        is_signed_today: false,
        start_time: 0,
        attendance_config_id: 1
      }
    ]
  };
}

module.exports = {
  endpoint: 'GetAttendanceList',
  reqType: null,
  resType: 'CSGetAttendanceListRes',
  handler: handleGetAttendanceList
};
