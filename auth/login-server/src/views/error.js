'use strict';

const { escapeHtml } = require('./_util');

// Generic error page rendered by the Discord callback route and by the
// global error handler. Content is plain-English and never exposes stack
// traces, request identifiers, or internal state.
module.exports = function renderErrorPage({ title, message }) {
  const safeTitle = escapeHtml(title);
  const safeMessage = escapeHtml(message);
  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>${safeTitle}</title>
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
      padding: 32px;
      max-width: 460px;
      width: 100%;
      text-align: center;
      box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
    }
    .badge {
      display: inline-block;
      width: 48px;
      height: 48px;
      line-height: 48px;
      border-radius: 50%;
      background: rgba(244, 67, 54, 0.12);
      color: #ff6b6b;
      font-size: 28px;
      font-weight: 700;
      margin-bottom: 16px;
    }
    h1 {
      margin: 0 0 12px 0;
      font-size: 20px;
      font-weight: 700;
      color: #fff;
    }
    p {
      color: #999;
      font-size: 14px;
      line-height: 1.5;
      margin: 0 0 16px 0;
    }
    .hint {
      margin-top: 24px;
      font-size: 12px;
      color: #666;
    }
  </style>
</head>
<body>
  <main>
    <div class="card">
      <div class="badge">!</div>
      <h1>${safeTitle}</h1>
      <p>${safeMessage}</p>
      <div class="hint">You can safely close this page.</div>
    </div>
  </main>
</body>
</html>`;
};
