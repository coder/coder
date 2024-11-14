//go:build darwin

package main

import "C"

import (
	"context"

	"golang.org/x/sys/unix"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/vpn"
)

// OpenTunnel creates a new VPN tunnel by `dup`ing the provided 'PIPE'
// file descriptors for reading, writing, and logging.
//
//export OpenTunnel
func OpenTunnel(cReadFD, cWriteFD int32) int32 {
	ctx := context.Background()

	readFD, err := unix.Dup(int(cReadFD))
	if err != nil {
		return -1
	}

	writeFD, err := unix.Dup(int(cWriteFD))
	if err != nil {
		unix.Close(readFD)
		return -1
	}

	conn, err := vpn.NewBidirectionalPipe(uintptr(cReadFD), uintptr(cWriteFD))
	if err != nil {
		unix.Close(readFD)
		unix.Close(writeFD)
		return -1
	}

	// Logs will be sent over the protocol
	_, err = vpn.NewTunnel(ctx, slog.Make(), conn)
	if err != nil {
		unix.Close(readFD)
		unix.Close(writeFD)
		return -1
	}

	return 0
}

func main() {}
