'use strict';

const config = require('./config');
const { logger, childForRequest } = require('./logger');
const mongo = require('./mongo');
const store = require('./store');
const ids = require('./ids');
const tokens = require('./tokens');
const signedRequest = require('./signed-request');
const oauthState = require('./oauth-state');
const { createRateLimiter } = require('./rate-limit');
const { createSecurityHeaders } = require('./security-headers');
const httpErrors = require('./http-errors');
const { createGuestsRepo } = require('./repositories/guests');
const { createAccountsRepo } = require('./repositories/accounts');
const { createPairingsRepo } = require('./repositories/pairings');

module.exports = {
  config,
  logger,
  childForRequest,
  mongo,
  store,

  // ids
  generateOpenId: ids.generateOpenId,
  nextUid: ids.nextUid,
  withUidRetry: ids.withUidRetry,
  FIRST_UID: ids.FIRST_UID,

  // tokens
  generateToken: tokens.generateToken,
  issueTokens: tokens.issueTokens,
  refreshAccessToken: tokens.refreshAccessToken,
  validateAccessToken: tokens.validateAccessToken,
  validateRefreshToken: tokens.validateRefreshToken,
  ACCESS_TTL_SECONDS: tokens.ACCESS_TTL_SECONDS,
  REFRESH_TTL_SECONDS: tokens.REFRESH_TTL_SECONDS,
  nowSeconds: tokens.nowSeconds,

  // signed_request
  buildSignedRequest: signedRequest.buildSignedRequest,
  verifySignedRequest: signedRequest.verifySignedRequest,
  base64UrlNoPad: signedRequest.base64UrlNoPad,

  // oauth state
  signState: oauthState.signState,
  verifyState: oauthState.verifyState,

  // middleware / errors
  createRateLimiter,
  createSecurityHeaders,
  oauthInvalidGrant: httpErrors.oauthInvalidGrant,
  graphOAuthError: httpErrors.graphOAuthError,
  asyncHandler: httpErrors.asyncHandler,

  // repositories
  createGuestsRepo,
  createAccountsRepo,
  createPairingsRepo,
};
