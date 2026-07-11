'use strict';

// Minimal HTML escaper for use inside attributes and text content. The
// login server does not reflect user input into HTML, so this is defense
// in depth rather than mitigation of a known injection surface.
function escapeHtml(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

module.exports = { escapeHtml };
