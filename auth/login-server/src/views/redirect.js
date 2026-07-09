'use strict';

const { escapeHtml } = require('./_util');

// Rendered on the browser that actually finished Discord OAuth. The WebView
// hosting the /dialog/oauth page has been polling in parallel and will
// pick up the completed pairing via /pair/status, so this page's only job
// is to tell the user they can return to the game.
//
// As a fallback for the (uncommon) case where the OAuth flow completed
// inside the game's WebView itself — in which case the polling loop is
// gone because the WebView navigated away — the page also carries:
//   - a <meta http-equiv="refresh"> pointing at fbconnect://...
//   - a "Return to game" anchor that the user can tap manually
// Both are inert in a stock system browser (custom-scheme handoff is
// blocked from cross-origin navigation there); both work inside the
// WebView.
module.exports = function renderCompletePage(redirectUrl) {
  const safeUrl = escapeHtml(redirectUrl);
  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta http-equiv="refresh" content="0; url=${safeUrl}">
  <title>Sign-in complete</title>
  <style>
    html, body { margin: 0; padding: 0; min-height: 100%; }
    body {
      font-family: system-ui, -apple-system, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      background: linear-gradient(135deg, #1a1a1a 0%, #0d0d0d 100%);
      color: #e0e0e0;
      display: flex;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
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
    .check {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 56px;
      height: 56px;
      border-radius: 50%;
      background: rgba(76, 175, 80, 0.12);
      color: #4CAF50;
      font-size: 32px;
      margin-bottom: 16px;
    }
    h1 {
      margin: 0 0 8px 0;
      font-size: 20px;
      font-weight: 700;
      color: #fff;
    }
    p {
      color: #999;
      font-size: 14px;
      line-height: 1.5;
      margin: 0 0 24px 0;
    }
    .btn {
      display: inline-block;
      background: #5865F2;
      color: #fff;
      text-decoration: none;
      padding: 12px 22px;
      border-radius: 8px;
      font-size: 14px;
      font-weight: 600;
      width: 100%;
      box-sizing: border-box;
    }
    .btn:hover { background: #4752C4; }
    .hint {
      margin-top: 16px;
      color: #555;
      font-size: 11px;
      line-height: 1.5;
    }
  </style>
</head>
<body>
  <div class="card">
    <div class="check">&#10003;</div>
    <h1>Sign-in complete</h1>
    <p>You can return to the game now.</p>
    <a class="btn" href="${safeUrl}">Return to game</a>
    <div class="hint">
      If the game did not resume automatically, close this tab. The game will pick up your sign-in in a moment.
    </div>
  </div>
</body>
</html>`;
};
