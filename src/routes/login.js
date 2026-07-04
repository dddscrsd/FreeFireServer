const express = require('express');
const router = express.Router();

router.post('/MajorRegister', (req, res) => {
    const data = req.body;
    // Process the login data (e.g., validate credentials, generate token, etc.)
    // For demonstration, we'll just return a success message with the received data
    res.json({
        account_id: 10000001,
        first_game_open: false,
        br_tutorial_open: false,
        cs_tutorial_open: false
    });
});

module.exports = router;