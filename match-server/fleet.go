package main

import (
	"log"
	"time"
)

// fleetHeartbeat keeps this instance registered in the match-server fleet registry so
// the matchmaker can allocate matches to it. addr is the PUBLIC host:port the client
// dials; the registry entry ages out (matchmaker stops picking it) if the heartbeat
// stops — i.e. the instance died. Runs for the life of the process.
func fleetHeartbeat(addr string) {
	for {
		if err := eventBus.RegisterServer(addr, time.Now().Unix()); err != nil {
			log.Printf("[mm-udp] fleet register err: %v", err)
		}
		time.Sleep(10 * time.Second)
	}
}
