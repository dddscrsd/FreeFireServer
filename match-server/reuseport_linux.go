//go:build linux

package main

import "syscall"

// reusePortSupported is true on Linux, where multiple UDP sockets can bind the same addr:port with
// SO_REUSEPORT and the kernel load-balances inbound datagrams across them (one per receive goroutine,
// so RX spreads over cores instead of bottlenecking on one). See listenUDPMulti.
const reusePortSupported = true

// controlReusePort is a net.ListenConfig.Control hook that sets SO_REUSEPORT on the listening socket
// before bind. SO_REUSEPORT is 0x0F on Linux (not exported by the std syscall package on all versions,
// so it's inlined here to avoid an extra dependency).
func controlReusePort(network, address string, c syscall.RawConn) error {
	var serr error
	if err := c.Control(func(fd uintptr) {
		serr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, 0x0F /* SO_REUSEPORT */, 1)
	}); err != nil {
		return err
	}
	return serr
}
