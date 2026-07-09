// Command bussmoke exercises the Redis event bus from the Go side. Needs a
// running Redis (REDIS_URL, default redis://127.0.0.1:6379).
//
//	go run ./cmd/bussmoke consume   # join the group + print Pings, stay up
//	go run ./cmd/bussmoke publish    # send one Ping to stream + pubsub, exit
//	go run ./cmd/bussmoke            # do both in one process
//
// Cross-language check: run this as the consumer and `node scripts/bus-smoke.js
// publish` from the repo root — this side should print the Node Ping.
package main

import (
	"log"
	"os"
	"time"

	"libmadoka/match-server/bus"
	"libmadoka/match-server/bus/pb"
)

func main() {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		url = "redis://127.0.0.1:6379"
	}
	mode := "both"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	b, err := bus.New(url, "go-smoke", "go-smoke-1")
	if err != nil {
		log.Fatalf("bus: %v", err)
	}
	defer b.Close()

	if mode == "consume" || mode == "both" {
		b.SubscribeStream("bus.ping", "smoke", "go-1", func(env *pb.Envelope) error {
			var p pb.Ping
			if err := bus.Decode(env, &p); err != nil {
				return err
			}
			log.Printf("[stream] from %s: from=%s nonce=%d note=%q", env.Source, p.From, p.Nonce, p.Note)
			return nil
		})
		b.SubscribePS("bus.ping", func(env *pb.Envelope) {
			var p pb.Ping
			if bus.Decode(env, &p) == nil {
				log.Printf("[pubsub] from %s: from=%s", env.Source, p.From)
			}
		})
	}

	if mode == "publish" || mode == "both" {
		time.Sleep(300 * time.Millisecond)
		b.Publish("bus.ping", &pb.Ping{From: "go", Nonce: time.Now().Unix() % 100000, Note: "hello from go"})
		b.PublishPS("bus.ping", &pb.Ping{From: "go", Nonce: 1, Note: "ps hello"})
		log.Printf("[publish] sent Ping on stream:bus.ping and ps:bus.ping")
	}

	if mode == "publish" {
		time.Sleep(300 * time.Millisecond)
		return
	}
	log.Printf("listening… (ctrl-c to exit)")
	select {}
}
