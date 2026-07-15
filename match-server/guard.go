package main

import (
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// DoS hardening for the public UDP listener. A direct flood of junk datagrams — e.g. packets that fail
// the msg-key/CRC decode (ErrMsgKey) — otherwise costs, PER packet, one log line + one heap allocation
// on the single receive goroutine (serve). That amplification is what actually hurts, not the decode
// itself. This layer keeps every dropped packet cheap and blocks a source that floods.
//
// IMPORTANT: the process still has to ReadFromUDP every datagram the kernel delivers — nothing in the
// app can drop a packet BEFORE that read. A large or SPOOFED-source flood must be filtered UPSTREAM:
// an nftables/iptables per-source rate limit on the UDP port, the cloud firewall/security group, or a
// UDP-aware DDoS scrubber. This layer blunts a direct single/few-source flood and removes the per-packet
// log + allocation cost so the loop can still drain legit traffic.

var droppedPackets atomic.Int64 // invalid / rate-limited datagrams since the last stats log (aggregated)

const (
	guardWindow     = time.Second      // per-source counting window
	guardMaxPerSec  = 400              // weighted datagrams/sec from one source IP before it's blocked (a legit client is ~35/s)
	guardBadWeight  = 4                // a junk (bad msg-key / undecodable) datagram counts this much toward the limit
	guardBlockDur   = 60 * time.Second // how long a flooding source is dropped
	guardMaxTracked = 8192             // cap on tracked source IPs, so a spoofed-source flood can't grow the map unbounded
)

type srcStat struct {
	winStart     time.Time
	weight       int
	blockedUntil time.Time
}

var (
	guardMu  sync.Mutex
	guardMap = make(map[string]*srcStat)
)

// allowSource reports whether a datagram from source IP `ip` should be processed. `bad` marks a datagram
// that already failed the cheap header check (junk) — those count 4x, so a junk flood trips the per-IP
// block far sooner than a legit client's ~35 pkt/s ever could. O(1); the map is capped + pruned.
func allowSource(ip string, bad bool) bool {
	now := time.Now()
	guardMu.Lock()
	defer guardMu.Unlock()
	st := guardMap[ip]
	if st == nil {
		if len(guardMap) >= guardMaxTracked {
			// Map full (spoofed-source flood): don't grow it. Still drop junk; let real packets through
			// — the untracked source just isn't individually rate-limited (upstream filtering must).
			return !bad
		}
		st = &srcStat{winStart: now}
		guardMap[ip] = st
	}
	if now.Before(st.blockedUntil) {
		return false
	}
	if now.Sub(st.winStart) >= guardWindow {
		st.winStart, st.weight = now, 0
	}
	w := 1
	if bad {
		w = guardBadWeight
	}
	st.weight += w
	if st.weight > guardMaxPerSec {
		st.blockedUntil = now.Add(guardBlockDur)
		log.Printf("[mm-udp] rate-limit: blocking flooding source %s for %s (weight=%d in <1s)", ip, guardBlockDur, st.weight)
		return false
	}
	return true
}

// guardMaintenance logs the aggregated drop count every 5s (instead of one line per dropped packet) and
// prunes idle / unblocked source entries so guardMap stays bounded. Runs as one background goroutine.
func guardMaintenance() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for range t.C {
		if d := droppedPackets.Swap(0); d > 0 {
			log.Printf("[mm-udp] dropped %d invalid/rate-limited datagrams in the last 5s", d)
		}
		now := time.Now()
		guardMu.Lock()
		for ip, st := range guardMap {
			if now.After(st.blockedUntil) && now.Sub(st.winStart) > guardWindow {
				delete(guardMap, ip) // idle and not blocked -> forget it
			}
		}
		guardMu.Unlock()
	}
}
