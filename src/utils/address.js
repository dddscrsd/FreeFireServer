const dgram = require('dgram');

async function getLocalIp() {
    return new Promise((resolve, reject) => {
        const socket = dgram.createSocket('udp4');

        socket.connect(53, '8.8.8.8', () => {
            const address = socket.address();
            socket.close();
            resolve(address.address);
        });

        socket.on('error', err => {
            socket.close();
            reject(err);
        });
    });
}

module.exports = { getLocalIp };