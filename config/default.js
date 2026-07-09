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
