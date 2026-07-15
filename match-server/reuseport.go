package main

import (
	"context"
	"net"
	"os"
	"runtime"
	"strconv"
)

// reusePortSockets returns how many receive sockets to open. With SO_REUSEPORT (Linux) that's one per
// CPU (capped), so inbound datagrams — and the work of dropping a flood — spread across cores instead
// of pinning one. REUSEPORT_SOCKETS overrides the count; off Linux it's always 1.
func reusePortSockets() int {
	if !reusePortSupported {
		return 1
	}
	n := runtime.NumCPU()
	if v := os.Getenv("REUSEPORT_SOCKETS"); v != "" {
		if x, err := strconv.Atoi(v); err == nil && x > 0 {
			n = x
		}
	}
	if n < 1 {
		n = 1
	}
	if n > 32 {
		n = 32
	}
	return n
}

// listenUDPMulti opens n UDP sockets bound to addr with SO_REUSEPORT so the kernel fans inbound
// datagrams across them (one receive goroutine each). n==1 (or off Linux) is a single plain socket —
// the previous behaviour. On any error it closes what it opened and returns the error.
func listenUDPMulti(addr string, n int) ([]*net.UDPConn, error) {
	lc := net.ListenConfig{Control: controlReusePort}
	conns := make([]*net.UDPConn, 0, n)
	for i := 0; i < n; i++ {
		pc, err := lc.ListenPacket(context.Background(), "udp", addr)
		if err != nil {
			for _, c := range conns {
				c.Close()
			}
			return nil, err
		}
		conns = append(conns, pc.(*net.UDPConn))
	}
	return conns, nil
}
