/**
 * CSVConfig  (-> Empty)
 * reference: handle_CSVConfig @ htpp.py:5964 — empty stub (returns empty body).
 * Ported from ported_2/ported_6 emptyStub; returns a default-constructed res.
 */

'use strict';

function emptyStub(reqObj, ctx) {
  ctx.logger.info('[ported_6] CSVConfig');
  return {};
}

module.exports = {
  endpoint: 'CSVConfig',
  reqType: null,
  resType: 'Empty',
  handler: emptyStub
};
