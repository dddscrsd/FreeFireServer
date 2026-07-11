#!/usr/bin/env sh
# Regenerate the Go bus bindings from the shared proto/bus/*.proto.
# The Node tier loads the same .proto at runtime via protobufjs — no codegen there,
# so this is the ONE source of truth for both languages.
#
# Requires: protoc, and protoc-gen-go on PATH:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   export PATH="$PATH:$(go env GOPATH)/bin"
set -e
cd "$(dirname "$0")/.."

mkdir -p match-server/bus/pb
protoc --proto_path=proto/bus \
  --go_out=paths=source_relative:match-server/bus/pb \
  proto/bus/envelope.proto proto/bus/events.proto

echo "generated match-server/bus/pb/{envelope,events}.pb.go"
