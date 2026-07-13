// Small env helpers for the auth-gate knobs below.
const _bool = (v, d) => (v == null || v === '' ? d : (v === '1' || String(v).toLowerCase() === 'true'));
const _list = (v) => String(v || '').split(',').map((s) => s.trim()).filter(Boolean);

module.exports = {
  port: 3000,
  ports: {
    live: Number(process.env.LIVE_PORT || process.env.PORT || 3000),
    login: Number(process.env.LOGIN_PORT || 3001),
    main: Number(process.env.MAIN_PORT || 3002),
    // TCP gateway (notification channel). Handed to the client as
    // notification_channel in GetLoginData; the client opens a persistent AES
    // socket here for server push (see protocol/TCP_PROTOCOL.md).
    tcp: Number(process.env.TCP_PORT || 10300)
  },
  version: '1.70.1',
  security: {
    cors: { origin: '*' },
    rateLimit: { windowMs: 15 * 60 * 1000, max: 100 }
  },
  protocol: {
    // How the AES ciphertext is carried in the HTTP body:
    //   'raw'    -> body is the raw ciphertext bytes (Content-Type application/octet-stream)
    //   'base64' -> body is a base64 string of the ciphertext
    bodyEncoding: process.env.BODY_ENCODING || 'raw',
    // Public URL handed back to the client in login responses (server_url etc.).
    serverUrl: process.env.SERVER_URL || '',
    // Realtime endpoints the client connects to AFTER GetLoginData (read from
    // LoginRes). Leave host empty to derive it from the incoming request host.
    // Point these at your TCP game/chat/notification servers when available.
    realtimeHost: process.env.REALTIME_HOST || '',     // host only, e.g. 192.168.1.10
    gameServerPort: process.env.GAME_SERVER_PORT || '10100',
    chatPort: process.env.CHAT_PORT || '10200',
    notificationPort: process.env.NOTI_PORT || '10300',
    defaultRegion: process.env.DEFAULT_REGION || 'US'
  },
  db: {
    // SQLite file for the accounts store (created on first run).
    file: process.env.DB_FILE || ''
  },
  match: {
    // Shared HMAC secret used to sign the match prepare_token (an HS256 JWT).
    // The Go match-server verifies with the SAME secret via its own
    // MATCH_JWT_SECRET env var — set both to the same value in production.
    jwtSecret: process.env.MATCH_JWT_SECRET || 'dev-match-secret-change-me',
    // prepare_token lifetime (seconds); the token is minted when a match forms
    // and consumed when the client posts cmd 440 to the game server.
    jwtTtlSec: Number(process.env.MATCH_JWT_TTL_SEC || 600)
  },
  // --- infra migration (Phase 0+) -------------------------------------------
  // Redis event bus (Streams + PubSub); consumed by the bus client in src/bus.
  redis: { url: process.env.REDIS_URL || 'redis://127.0.0.1:6379' },
  // PostgreSQL (Phase 1+). Empty keeps the SQLite path (db.file) in use.
  postgres: { url: process.env.DATABASE_URL || process.env.DB_URL || '' },
  // --- auth / login hardening ----------------------------------------------
  // Gates the game login/register against the auth server's store + the client's
  // signature/version. Every gate is OFF or safe by default so the live (incl.
  // guest) client keeps working; turn them on per-deployment once the exact
  // client values are known. See src/handlers/_authGate.js and src/db/authStore.js.
  auth: {
    // Postgres schema the auth server writes to (must match AUTH_PG_SCHEMA there).
    pgSchema: process.env.AUTH_PG_SCHEMA || 'auth',
    // Only accounts registered on the auth server (open_id present in auth.guests)
    // may log in / register. OFF by default: enabling it blocks any client whose
    // open_id isn't provisioned by the auth server (including the dev guest unless
    // allowGuest is on). Requires DATABASE_URL to point at the auth Postgres.
    enforceRegistration: _bool(process.env.AUTH_ENFORCE_REGISTRATION, false),
    // When enforcing registration, also require the presented login_token to equal
    // the auth store's access_token (not just that the open_id exists).
    enforceLoginToken: _bool(process.env.AUTH_ENFORCE_LOGIN_TOKEN, false),
    // Let the deterministic dev guest (GUEST_OPEN_ID) bypass the registration gate
    // so guest testing still works while enforcement is on for real accounts.
    allowGuest: _bool(process.env.AUTH_ALLOW_GUEST, true),
    guestOpenId: process.env.GUEST_OPEN_ID || 'guest-default',
    // client_version allow-list. Empty => gate OFF (any version). Set to the exact
    // string(s) your client sends (logged at login) to reject wrong versions.
    allowedVersions: _list(process.env.ALLOWED_CLIENT_VERSIONS),
    // signature_md5 allow-list. Empty => gate OFF. When set, a login whose
    // signature_md5 isn't listed is refused (403) AND the account is TCP-kicked.
    allowedSignatures: _list(process.env.ALLOWED_SIGNATURE_MD5),
    // Enforce account bans (auth.accounts.banned / authorized=false, or the game
    // store's state.banned). ON by default — harmless until an account is flagged.
    enforceBans: _bool(process.env.AUTH_ENFORCE_BANS, true),
  },
  // Per-module public domains: routed by the edge proxy and handed to the client
  // (server_url / GetLoginData / match handoff). Empty => derive from request host.
  domains: {
    live: process.env.LIVE_DOMAIN || '',
    login: process.env.LOGIN_DOMAIN || '',
    main: process.env.MAIN_DOMAIN || '',
    tcp: process.env.TCP_PUBLIC_HOST || '',
    match: process.env.MATCH_PUBLIC_HOST || ''
  },
  // This process/instance id — presence keys, bus consumer names, match allocation.
  nodeId: process.env.NODE_ID || require('os').hostname()
};
