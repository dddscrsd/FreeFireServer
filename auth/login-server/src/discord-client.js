'use strict';

const { config } = require('@auth/shared');

const TIMEOUT_MS = 10_000;
const TOKEN_URL = 'https://discord.com/api/oauth2/token';
const USER_URL = 'https://discord.com/api/users/@me';

// Typed error surfaced by both API calls. The stage lets the caller decide
// which error page to render without inspecting the message string.
class DiscordApiError extends Error {
  constructor(message, { stage, status, body } = {}) {
    super(message);
    this.name = 'DiscordApiError';
    this.stage = stage;
    this.status = status;
    this.bodyPreview = body;
  }
}

async function readBodyPreview(response, limit = 200) {
  try {
    const text = await response.text();
    return text.slice(0, limit);
  } catch {
    return '';
  }
}

// Exchange the authorization code for a Discord access token. The returned
// token is used exactly once, immediately, to fetch the user's profile.
// It is never persisted anywhere.
async function exchangeCode(code) {
  const body = new URLSearchParams({
    grant_type: 'authorization_code',
    code,
    redirect_uri: config.DISCORD_REDIRECT_URI,
    client_id: config.DISCORD_CLIENT_ID,
    client_secret: config.DISCORD_CLIENT_SECRET,
  });

  let response;
  try {
    response = await fetch(TOKEN_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: body.toString(),
      signal: AbortSignal.timeout(TIMEOUT_MS),
    });
  } catch (err) {
    throw new DiscordApiError('discord token exchange network error', {
      stage: 'token',
      body: err && err.name ? err.name : 'network',
    });
  }
  if (!response.ok) {
    throw new DiscordApiError('discord token exchange failed', {
      stage: 'token',
      status: response.status,
      body: await readBodyPreview(response),
    });
  }
  let data;
  try {
    data = await response.json();
  } catch (err) {
    throw new DiscordApiError('discord token response was not json', {
      stage: 'token',
      status: response.status,
      body: err.message,
    });
  }
  if (!data.access_token || typeof data.access_token !== 'string') {
    throw new DiscordApiError('discord token response missing access_token', {
      stage: 'token',
      status: response.status,
    });
  }
  return { accessToken: data.access_token, tokenType: data.token_type || 'Bearer' };
}

// Fetch the Discord user's profile. Returns id, username, and email.
// email may be null when the user's Discord email is unverified.
async function fetchUser(discordAccessToken) {
  let response;
  try {
    response = await fetch(USER_URL, {
      headers: { Authorization: `Bearer ${discordAccessToken}` },
      signal: AbortSignal.timeout(TIMEOUT_MS),
    });
  } catch (err) {
    throw new DiscordApiError('discord user fetch network error', {
      stage: 'user',
      body: err && err.name ? err.name : 'network',
    });
  }
  if (!response.ok) {
    throw new DiscordApiError('discord user fetch failed', {
      stage: 'user',
      status: response.status,
      body: await readBodyPreview(response),
    });
  }
  let data;
  try {
    data = await response.json();
  } catch (err) {
    throw new DiscordApiError('discord user response was not json', {
      stage: 'user',
      status: response.status,
      body: err.message,
    });
  }
  if (!data.id || typeof data.id !== 'string') {
    throw new DiscordApiError('discord user response missing id', {
      stage: 'user',
      status: response.status,
    });
  }
  return {
    id: data.id,
    username: typeof data.username === 'string' ? data.username : String(data.id),
    email: typeof data.email === 'string' ? data.email : null,
  };
}

module.exports = { exchangeCode, fetchUser, DiscordApiError };
