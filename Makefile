.PHONY: proto bus-smoke-node bus-smoke-go

# Regenerate the Go bus protobuf bindings from proto/bus/*.proto.
# (Node loads the .proto at runtime — no codegen there.)
proto:
	sh scripts/gen-proto.sh

# Smoke tests — need a running Redis (REDIS_URL, default redis://127.0.0.1:6379).
bus-smoke-node:
	node scripts/bus-smoke.js

bus-smoke-go:
	cd match-server && go run ./cmd/bussmoke
