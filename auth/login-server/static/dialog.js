/* eslint-disable */
// Polls /pair/status while the user completes Discord OAuth in whichever
// browser opened the authorize link. When the server reports completion,
// this WebView-local navigation to fbconnect://success#... is what the
// game's URL scheme handler intercepts.
//
// This file is loaded via <script src="/static/dialog.js"> from the login
// page. Kept ES5-ish so older WebViews (Android 6/7) run it fine.
(function () {
  'use strict';

  var body = document.body;
  var pairId = body && body.dataset ? body.dataset.pairId : null;
  var statusEl = document.getElementById('status');
  var spinnerEl = document.getElementById('spinner');

  var POLL_INTERVAL_MS = 2000;
  var POLL_TIMEOUT_MS = 10 * 60 * 1000;    // must be <= pairings TTL
  var startedAt = Date.now();

  function setStatus(text, mood) {
    if (!statusEl) return;
    statusEl.textContent = text;
    statusEl.className = 'status ' + (mood || '');
  }

  function stopSpinner() {
    if (spinnerEl && spinnerEl.parentNode) spinnerEl.parentNode.removeChild(spinnerEl);
  }

  function finish(msg, mood) {
    stopSpinner();
    setStatus(msg, mood || 'error');
  }

  function poll() {
    if (Date.now() - startedAt > POLL_TIMEOUT_MS) {
      finish('Sign-in timed out. Reload this page to try again.');
      return;
    }

    fetch('/pair/status?id=' + encodeURIComponent(pairId), { cache: 'no-store' })
      .then(function (r) { return r.ok ? r.json() : { status: 'error' }; })
      .then(function (data) {
        if (data && data.status === 'completed' && typeof data.redirect_url === 'string') {
          // We stay in the WebView by design: the game intercepts the
          // fbconnect:// scheme from within its own webview navigation,
          // not from the system browser.
          window.location.href = data.redirect_url;
          return;
        }
        if (data && data.status === 'failed') {
          finish('Sign-in failed. Reload this page to try again.');
          return;
        }
        if (data && data.status === 'expired') {
          finish('Sign-in session expired. Reload this page to try again.');
          return;
        }
        // pending / unknown → keep polling
        window.setTimeout(poll, POLL_INTERVAL_MS);
      })
      .catch(function () {
        // Network hiccup — keep trying. Rate limiter will smooth this.
        window.setTimeout(poll, POLL_INTERVAL_MS);
      });
  }

  if (pairId) {
    window.setTimeout(poll, POLL_INTERVAL_MS);
  } else {
    finish('Sign-in session is missing. Reload this page.');
  }
})();
