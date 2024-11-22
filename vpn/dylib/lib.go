//go:build darwin

package main

import "C"

import (
	"context"

	"golang.org/x/sys/unix"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/vpn"
)

const (
	ErrDupReadFD  = -2
	ErrDupWriteFD = -3
	ErrOpenPipe   = -4
	ErrNewTunnel  = -5
)

// OpenTunnel creates a new VPN tunnel by `dup`ing the provided 'PIPE'
// file descriptors for reading, writing, and logging.
//
//export OpenTunnel
func OpenTunnel(cReadFD, cWriteFD int32) int32 {
	ctx := context.Background()

	readFD, err := unix.Dup(int(cReadFD))
	if err != nil {
		return ErrDupReadFD
	}

	writeFD, err := unix.Dup(int(cWriteFD))
	if err != nil {
		unix.Close(readFD)
		return ErrDupWriteFD
	}

	conn, err := vpn.NewBidirectionalPipe(uintptr(cReadFD), uintptr(cWriteFD))
	if err != nil {
		unix.Close(readFD)
		unix.Close(writeFD)
		return ErrOpenPipe
	}

	// Logs will be sent over the protocol
	_, err = vpn.NewTunnel(ctx, slog.Make(), conn)
	if err != nil {
		unix.Close(readFD)
		unix.Close(writeFD)
		return ErrNewTunnel
	}

	return 0
}

func main() {}
