'use strict';

const HTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Privacy Policy</title>
  <style>
    html, body { margin: 0; padding: 0; }
    body {
      font-family: system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
      background: linear-gradient(135deg, #1a1a1a 0%, #0d0d0d 100%);
      color: #ccc;
      line-height: 1.6;
      padding: 32px 20px 48px;
    }
    .container { max-width: 780px; margin: 0 auto; }
    h1 { color: #fff; font-size: 24px; margin-bottom: 24px; }
    h2 { color: #ff6600; font-size: 15px; text-transform: uppercase; letter-spacing: 1px; margin-top: 28px; margin-bottom: 10px; }
    p, li { font-size: 14px; margin: 10px 0; }
    ul { padding-left: 24px; }
    .disclaimer {
      background: rgba(0, 150, 255, 0.08);
      border: 1px solid rgba(0, 150, 255, 0.3);
      border-radius: 8px;
      padding: 14px 16px;
      margin-bottom: 24px;
    }
    .disclaimer p { color: #7ac8ff; margin: 0; font-size: 13px; }
    a { color: #ff6600; text-decoration: none; }
    a:hover { text-decoration: underline; }
    .back { display: inline-block; margin-top: 32px; color: #888; font-size: 13px; }
  </style>
</head>
<body>
  <div class="container">
    <h1>Privacy Policy</h1>
    <div class="disclaimer">
      <p>This is an independent community project. It is not affiliated with Garena or Free Fire.</p>
    </div>
    <h2>What we collect</h2>
    <p>When you sign in with Discord we receive and store:</p>
    <ul>
      <li>Your Discord user id.</li>
      <li>Your Discord username (handle).</li>
      <li>Your Discord email address, if you have granted the email scope.</li>
      <li>The IP address that initiated the sign-in.</li>
      <li>Timestamps for account creation and each sign-in.</li>
    </ul>
    <h2>How we use it</h2>
    <p>Your data is used to identify your account, to authenticate you when you return, and to protect the service from abuse. We do not sell, share, or trade your data with third parties.</p>
    <h2>Discord tokens</h2>
    <p>The service does not persist Discord access tokens. Discord tokens are used once during sign-in to fetch your profile and are then discarded.</p>
    <h2>Cookies</h2>
    <p>The sign-in flow does not set any authentication cookies. The game client uses a bearer token returned in the redirect fragment.</p>
    <h2>Retention</h2>
    <p>Account records are retained for as long as your account exists. Contact the operator to request deletion.</p>
    <h2>Contact</h2>
    <p>For privacy questions, contact the service operator.</p>
    <a class="back" href="/">&larr; Back</a>
  </div>
</body>
</html>`;

module.exports = function renderPrivacy() {
  return HTML;
};
