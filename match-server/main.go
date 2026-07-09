package main

import (
	"encoding/hex"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"libmadoka/match-server/bus"
	"libmadoka/match-server/packet"
)

// secretHex is the stub match secret — must match the matchmaker's SussNtf.secret
// (16-byte TEA key).
const secretHex = "00112233445566778899aabbccddeeff"

// Active sessions, keyed by remote UDP address. Package-level so the shutdown handler
// can kick every connected client. Guarded by sessionsMu.
var (
	sessions   = map[string]*session{}
	sessionsMu sync.Mutex
)

// eventBus is the process-wide Redis event bus, or nil when REDIS_URL is unset or
// unreachable — the match server then runs standalone (unchanged). Match end
// publishes the durable settlement events (match.result / match.ended) through it;
// see results.go.
var eventBus *bus.Bus

func main() {
	addr := flag.String("addr", ":10100", "UDP listen address")
	flag.Parse()
	cfg = loadConfig()

	// Connect the event bus if configured. A missing/unreachable Redis is non-fatal:
	// the match server logs it and runs standalone (results just won't be settled).
	if cfg.redisURL != "" {
		if b, err := bus.New(cfg.redisURL, "match", cfg.nodeID); err != nil {
			log.Printf("[mm-udp] bus disabled (redis unreachable): %v", err)
		} else {
			eventBus = b
			log.Printf("[mm-udp] bus connected: node=%s", cfg.nodeID)
		}
	}

	// Advertise this instance in the fleet registry so the matchmaker can allocate
	// matches to it. MATCH_ADVERTISE = the PUBLIC host:port the client dials for THIS
	// instance; without it the matchmaker falls back to its static configured address.
	if eventBus != nil && cfg.advertiseAddr != "" {
		go fleetHeartbeat(cfg.advertiseAddr)
		log.Printf("[mm-udp] fleet register: advertise=%s", cfg.advertiseAddr)
	}

	key, err := hex.DecodeString(secretHex)
	if err != nil {
		log.Fatalf("bad secret: %v", err)
	}
	udpAddr, err := net.ResolveUDPAddr("udp", *addr)
	if err != nil {
		log.Fatalf("resolve: %v", err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer conn.Close()
	log.Printf("[mm-udp] match server listening on %s (udp)", conn.LocalAddr())

	// On shutdown, kick every client so they drop to the lobby instead of reconnecting
	// into a dead/stale match. Triggered by a signal (Ctrl+C / SIGTERM) or, for the
	// dev loop where a background process can't easily be signalled on Windows, by a
	// stop-file (MATCH_STOP_FILE).
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; kickAllAndExit("signal") }()
	if p := os.Getenv("MATCH_STOP_FILE"); p != "" {
		go watchStopFile(p)
	}

	serve(conn, key)
}

// serve is the receive loop: decode each datagram, look up (or create) its session,
// and dispatch it.
func serve(conn *net.UDPConn, key []byte) {
	buf := make([]byte, 8192)
	for {
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("[mm-udp] read err: %v", err)
			continue
		}
		p, err := packet.Decode(append([]byte(nil), buf[:n]...), key)
		if err != nil {
			log.Printf("[mm-udp] drop from %v (%dB): %v", remote, n, err)
			continue
		}
		raddr := remote.String()
		sessionsMu.Lock()
		s := sessions[raddr]
		if s == nil {
			s = &session{conn: conn, remote: remote, key: key, out: newWriter(conn, remote, key)}
			sessions[raddr] = s
			log.Printf("[mm-udp] new session %v", remote)
		}
		sessionsMu.Unlock()
		if p.Cmd != packet.CmdClientPos && p.Cmd != packet.CmdPing && p.Cmd != packet.CmdSyncServerTime {
			log.Printf("[mm-udp] recv %v cmd=%d so=%d seq=%d order=%d flags=%d len=%d",
				remote, p.Cmd, p.SendOption, p.SeqID, p.OrderID, p.Flags, len(p.Payload))
		}
		s.dispatch(p)
	}
}

// kickAllAndExit sends every connected client a SendKick (so=3) packet and exits, so a
// server restart drops clients to the lobby rather than leaving them reconnecting into
// a stale match (which breaks HP/kill state).
func kickAllAndExit(reason string) {
	sessionsMu.Lock()
	n := 0
	for _, s := range sessions {
		s.kick()
		n++
	}
	sessionsMu.Unlock()
	log.Printf("[mm-udp] shutdown (%s): kicked %d client(s) via SendKick (so=3)", reason, n)
	time.Sleep(150 * time.Millisecond) // let the UDP kicks flush before the process dies
	os.Exit(0)
}

// watchStopFile polls for a dev stop-file; when it appears it kicks all clients and
// exits (then the file is consumed). Lets the dev loop trigger a graceful, kicking
// shutdown of a backgrounded server that can't be signalled directly on Windows.
func watchStopFile(path string) {
	for {
		if _, err := os.Stat(path); err == nil {
			os.Remove(path)
			kickAllAndExit("stop-file " + path)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// dispatch acks a reliable packet immediately (stateless + prompt, so the client doesn't resend),
// then routes it. Once the match loop is running, the handler is ENQUEUED so it executes on run()'s
// goroutine — the single owner of match state (Step 3b); the ping echo and the pre-match join/hello
// handlers run inline on the receive goroutine. Panic recovery wraps the inline work here; the
// enqueued handler carries its own (see enqueue), so one bad packet can't kill the server or the
// match loop.
func (s *session) dispatch(p *packet.Packet) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[mm-udp] dispatch panic: %v", r)
		}
	}()
	if packet.IsReliable(p.Cmd, p.SendOption) {
		s.ack(p.SeqID)
	}
	if p.Cmd == packet.CmdPing { // stateless echo — keep it prompt, don't inflate the client's ping
		s.handlePing(p)
		return
	}
	if s.syncStarted { // the match loop owns the state — process the handler on its goroutine
		s.enqueue(func() { s.route(p) })
		return
	}
	s.route(p) // pre-match (join/hello): run inline, run() isn't up yet
}

// enqueue hands a handler to the match loop (run()) so it mutates match state on the single owning
// goroutine. The handler runs with its own panic recovery. If the mailbox is somehow full (run()
// wedged), it drops + logs rather than blocking the receive loop — which is shared by every client.
func (s *session) enqueue(fn func()) {
	wrapped := func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[mm-udp] handler panic: %v", r)
			}
		}()
		fn()
	}
	select {
	case s.match.mailbox <- wrapped:
	default:
		log.Printf("[mm-udp] mailbox full — dropped a handler for %v", s.remote)
	}
}
