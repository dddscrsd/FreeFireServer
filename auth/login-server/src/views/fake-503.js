'use strict';

// Verbatim copy of the fake unavailability page from the original Flask
// server. Returned by /dialog/oauth when the caller does not provide the
// expected client_id or redirect_uri, hiding the endpoint from casual
// scans.
const HTML = `<!DOCTYPE html>
<html>
<head><title>503 Service Unavailable</title></head>
<body>
<h1>503 Service Unavailable</h1>
<p>No server is available to handle this request.</p>
</body>
</html>`;

module.exports = function renderFake503() {
  return HTML;
};
