'use strict';

const { escapeHtml } = require('./_util');

// The login page renders "Continue with Discord" and immediately starts
// polling /pair/status via /static/dialog.js. When the user completes the
// Discord flow — in this WebView or in the device's system browser — the
// callback route marks the pairing completed and this page picks up the
// fbconnect://... URL and navigates the WebView to it.
module.exports = function renderLoginPage({ discordAuthorizeUrl, pairId }) {
  const href = escapeHtml(discordAuthorizeUrl);
  const pair = escapeHtml(pairId);
  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Sign in</title>
  <style>
    html, body { margin: 0; padding: 0; min-height: 100%; }
    body {
      font-family: system-ui, -apple-system, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      background: linear-gradient(135deg, #1a1a1a 0%, #0d0d0d 100%);
      color: #e0e0e0;
      display: flex;
      flex-direction: column;
      min-height: 100vh;
    }
    .disclaimer {
      background: rgba(0, 0, 0, 0.6);
      color: #999;
      font-size: 12px;
      text-align: center;
      padding: 10px 16px;
      border-bottom: 1px solid #2a2a2a;
      line-height: 1.4;
    }
    .disclaimer strong { color: #ff6600; }
    main {
      flex: 1;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 24px;
    }
    .card {
      background: #1a1a1a;
      border: 1px solid #2a2a2a;
      border-radius: 12px;
      padding: 40px 32px;
      max-width: 400px;
      width: 100%;
      text-align: center;
      box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
    }
    h1 {
      margin: 0 0 8px 0;
      font-size: 22px;
      font-weight: 700;
      letter-spacing: 1px;
      text-transform: uppercase;
      color: #fff;
    }
    .subtitle {
      color: #888;
      font-size: 13px;
      margin-bottom: 24px;
    }
    .btn {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 12px;
      background: #5865F2;
      color: #fff;
      text-decoration: none;
      padding: 14px 24px;
      border-radius: 8px;
      font-size: 15px;
      font-weight: 600;
      width: 100%;
      box-sizing: border-box;
      transition: background 0.15s ease;
    }
    .btn:hover { background: #4752C4; }
    .btn svg { flex-shrink: 0; }
    .waiting {
      margin-top: 20px;
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 10px;
      color: #666;
      font-size: 13px;
      min-height: 22px;
    }
    #spinner {
      width: 14px;
      height: 14px;
      border-radius: 50%;
      border: 2px solid #333;
      border-top-color: #5865F2;
      animation: spin 0.9s linear infinite;
    }
    @keyframes spin { to { transform: rotate(360deg); } }
    .status.error { color: #ff6b6b; }
    .hint {
      margin-top: 18px;
      color: #555;
      font-size: 11px;
      line-height: 1.5;
    }
    footer {
      text-align: center;
      padding: 16px 0 24px;
      font-size: 11px;
      color: #555;
    }
    footer a { color: #666; text-decoration: none; margin: 0 6px; }
    footer a:hover { color: #ff6600; }
  </style>
</head>
<body data-pair-id="${pair}">
  <div class="disclaimer">
    This is an <strong>independent community project</strong>. Not affiliated with, endorsed by, or connected to <strong>Garena</strong> or <strong>Free Fire</strong>.
  </div>
  <main>
    <div class="card">
      <h1>Sign In</h1>
      <div class="subtitle">Continue with your Discord account to enter the game.</div>
      <a class="btn" href="${href}">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
          <path d="M20.317 4.37a19.79 19.79 0 0 0-4.885-1.515.074.074 0 0 0-.079.037c-.21.375-.444.864-.608 1.25a18.27 18.27 0 0 0-5.487 0 12.64 12.64 0 0 0-.617-1.25.077.077 0 0 0-.079-.037A19.736 19.736 0 0 0 3.677 4.37a.07.07 0 0 0-.032.027C.533 9.046-.32 13.58.099 18.057a.082.082 0 0 0 .031.057 19.9 19.9 0 0 0 5.993 3.03.078.078 0 0 0 .084-.028 14.09 14.09 0 0 0 1.226-1.994.076.076 0 0 0-.041-.106 13.107 13.107 0 0 1-1.872-.892.077.077 0 0 1-.008-.128 10.2 10.2 0 0 0 .372-.292.074.074 0 0 1 .077-.01c3.928 1.793 8.18 1.793 12.062 0a.074.074 0 0 1 .078.01c.12.098.246.198.373.292a.077.077 0 0 1-.006.127 12.299 12.299 0 0 1-1.873.892.077.077 0 0 0-.041.107c.36.698.772 1.362 1.225 1.993a.076.076 0 0 0 .084.028 19.839 19.839 0 0 0 6.002-3.03.077.077 0 0 0 .032-.054c.5-5.177-.838-9.674-3.549-13.66a.061.061 0 0 0-.031-.03zM8.02 15.33c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.956-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.956 2.418-2.157 2.418zm7.975 0c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.955-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.946 2.418-2.157 2.418z"/>
        </svg>
        Continue with Discord
      </a>
      <div class="waiting">
        <div id="spinner"></div>
        <div id="status" class="status">Waiting for sign-in&hellip;</div>
      </div>
      <div class="hint">
        Keep this page open. If Discord opens in a different browser, come back here after you finish; the game will resume automatically.
      </div>
    </div>
  </main>
  <footer>
    <a href="/terms">Terms</a> &middot; <a href="/privacy">Privacy</a>
  </footer>
  <script src="/static/dialog.js"></script>
</body>
</html>`;
};
