//go:build !windows

package main

import (
	"fmt"
	"syscall"
)

// listenOnRandomPort opens a TCP listening socket on 127.0.0.1 with the
// given backlog. The socket is never accepted from. Returns the listening
// address (host:port) and a close func.
func listenOnRandomPort(backlog int) (string, func() error, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if err != nil {
		return "", nil, fmt.Errorf("socket: %w", err)
	}
	closer := func() error { return syscall.Close(fd) }

	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		_ = closer()
		return "", nil, fmt.Errorf("setsockopt SO_REUSEADDR: %w", err)
	}
	if err := syscall.Bind(fd, &syscall.SockaddrInet4{Port: 0, Addr: [4]byte{127, 0, 0, 1}}); err != nil {
		_ = closer()
		return "", nil, fmt.Errorf("bind: %w", err)
	}
	if err := syscall.Listen(fd, backlog); err != nil {
		_ = closer()
		return "", nil, fmt.Errorf("listen: %w", err)
	}
	sa, err := syscall.Getsockname(fd)
	if err != nil {
		_ = closer()
		return "", nil, fmt.Errorf("getsockname: %w", err)
	}
	addr := sa.(*syscall.SockaddrInet4)
	return fmt.Sprintf("127.0.0.1:%d", addr.Port), closer, nil
}
