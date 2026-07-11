// Smoke test for the Redis event bus (Node side). Needs a running Redis:
//   docker compose up -d redis      (or any local redis on REDIS_URL)
//
//   node scripts/bus-smoke.js consume   # join the group + print Pings, stay up
//   node scripts/bus-smoke.js publish    # send one Ping to stream + pubsub, exit
//   node scripts/bus-smoke.js            # do both in one process
//
// Cross-language check: run the Go consumer (`go run ./cmd/bussmoke consume` in
// match-server/) and then `node scripts/bus-smoke.js publish` — the Go side
// should print the Node Ping, proving the shared envelope contract works.
const { Bus } = require('../src/bus');

const url = process.env.REDIS_URL || 'redis://127.0.0.1:6379';
const mode = process.argv[2] || 'both';
const bus = new Bus({ url, source: 'node-smoke', node: 'node-smoke-1' });

async function main() {
  if (mode === 'consume' || mode === 'both') {
    bus.subscribeStream('bus.ping', 'smoke', 'node-1', (env) => {
      const p = Bus.payload(env, 'Ping');
      console.log(`[stream] from ${env.source}: from=${p.from} nonce=${p.nonce} note=${JSON.stringify(p.note)}`);
    });
    bus.subscribePS('bus.ping', (env) => {
      const p = Bus.payload(env, 'Ping');
      console.log(`[pubsub] from ${env.source}: from=${p.from}`);
    });
  }

  if (mode === 'publish' || mode === 'both') {
    await new Promise((r) => setTimeout(r, 300)); // let the group/subscription register
    await bus.publish('bus.ping', 'Ping', { from: 'node', nonce: Date.now() % 100000, note: 'hello from node' });
    await bus.publishPS('bus.ping', 'Ping', { from: 'node', nonce: 1, note: 'ps hello' });
    console.log('[publish] sent Ping on stream:bus.ping and ps:bus.ping');
  }

  if (mode === 'publish') {
    await new Promise((r) => setTimeout(r, 200));
    await bus.close();
  } else {
    console.log('listening… (ctrl-c to exit)');
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
