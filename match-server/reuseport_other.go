//go:build !linux

package main

import "syscall"

// Off Linux (e.g. the Windows dev box) SO_REUSEPORT isn't available in the same form, so the server
// falls back to a single listen socket. These stubs let the shared code in reuseport.go compile.
const reusePortSupported = false

func controlReusePort(network, address string, c syscall.RawConn) error { return nil }
