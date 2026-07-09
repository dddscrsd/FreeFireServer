'use strict';

const HTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Terms of Service</title>
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
    p { font-size: 14px; margin: 10px 0; }
    .disclaimer {
      background: rgba(220, 53, 69, 0.08);
      border: 1px solid rgba(220, 53, 69, 0.3);
      border-radius: 8px;
      padding: 14px 16px;
      margin-bottom: 24px;
    }
    .disclaimer p { color: #ff8a8a; margin: 0; font-size: 13px; }
    .disclaimer strong { color: #ff6b6b; }
    a { color: #ff6600; text-decoration: none; }
    a:hover { text-decoration: underline; }
    .back { display: inline-block; margin-top: 32px; color: #888; font-size: 13px; }
  </style>
</head>
<body>
  <div class="container">
    <h1>Terms of Service</h1>
    <div class="disclaimer">
      <p><strong>Notice:</strong> This is an <strong>independent community project</strong>. It is not affiliated with, endorsed by, or connected to <strong>Garena</strong>, <strong>Free Fire</strong>, or any of their subsidiaries. All trademarks referenced are the property of their respective owners.</p>
    </div>
    <h2>1. Acceptance</h2>
    <p>By using this service you agree to these terms. If you do not agree, do not use the service.</p>
    <h2>2. Nature of the service</h2>
    <p>The service is provided as-is for educational and community purposes. Availability, features, and data are not guaranteed and may change at any time.</p>
    <h2>3. Discord authentication</h2>
    <p>Sign-in relies on Discord OAuth. By signing in you allow this service to store your Discord user id, handle, and email so an account can be provisioned and looked up on subsequent logins.</p>
    <h2>4. Prohibited use</h2>
    <p>Do not attempt to bypass authentication, abuse the service, or use it for any illegal purpose.</p>
    <h2>5. Limitation of liability</h2>
    <p>The service is provided without warranty. The operators are not liable for any damages arising from the use or inability to use the service.</p>
    <h2>6. Changes</h2>
    <p>These terms may change without notice. Continued use of the service after changes constitutes acceptance of the revised terms.</p>
    <a class="back" href="/">&larr; Back</a>
  </div>
</body>
</html>`;

module.exports = function renderTerms() {
  return HTML;
};
