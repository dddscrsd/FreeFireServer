const express = require('express');
const router = express.Router();
const service = require('../services/exampleService');
const config = require('../../config/default');
// ver.php redirects the client to the LOGIN server (port 3001) next, so
// server_url below points there, not at this live server (3000).
const loginPort = config.ports.login;
// When LOGIN_DOMAIN is set (edge / Traefik), point the client at that domain over
// TLS; otherwise fall back to the derived LAN IP + login port (dev / no edge).
// A bare domain gets https://; include a scheme in the env to override it.
const loginDomain = (config.domains && config.domains.login) || '';
const version = process.env.GAME_VERSION || config.version || '1.70.0';
const { getLocalIp } = require('../utils/address');

function loginServerUrl(localIp) {
    if (!loginDomain) return 'http://' + localIp + ':' + loginPort + '/';
    const base = loginDomain.includes('://') ? loginDomain : 'https://' + loginDomain;
    return base.endsWith('/') ? base : base + '/';
}

router.get('/ver.php', async (req, res) => {
    const requestIp = req.headers['x-forwarded-for'] || req.ip;
    const localIp = loginDomain ? '' : await getLocalIp();
    data = {
        "appstore_url": "https://play.google.com/store/apps/details?id=com.dts.freefireth",
        "billboard_msg": "",
        "cdn_url": "https://dl.cdn.freefiremobile.com/live/ABHotUpdates/",
        "client_ip": requestIp,
        "code": 0,
        "country_code": "BR",
        "force_to_restart_app": false,
        "gdpr_version": 2,
        "is_firewall_open": false,
        "is_review_server": false,
        "is_server_open": true,
        "maintenance_announcement": "",
        "maintenance_region": "",
        "remote_option_version": "optionallocres:26|optionalclothres:282|optionalfullscreencgres:19|optionalludores:19|optionalmap1res:194|optionalmap2res:36|optionalmap4res:19|optionalmapres:17|optionalpetres:17|optionalrushb:38|optionalrushingpetsres:61|optionalvoiceres:147|optionalwerewolves:48",
        "remote_version": version,
        "server_url": loginServerUrl(localIp)
    }
    res.json(data);
});

module.exports = router;
