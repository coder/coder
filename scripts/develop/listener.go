//go:build !windows

package main

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

const (
	// procNetTCPListen is the state column of a socket in the LISTEN
	// state in /proc/net/tcp and /proc/net/tcp6.
	procNetTCPListen = "0A"
	procNetTCP       = "/proc/net/tcp"
	procNetTCP6      = "/proc/net/tcp6"
)

// loopbackListenAddrs holds the local addresses, as /proc/net/tcp and
// /proc/net/tcp6 print them, of a listening socket that accepts a
// connection to 127.0.0.1: 127.0.0.1, 0.0.0.0, :: and ::ffff:127.0.0.1.
// The kernel prints each 32-bit word of an address in host byte order, so
// these strings are for little-endian hosts (amd64 and arm64).
var loopbackListenAddrs = map[string]bool{
	"0100007F":                         true,
	"00000000":                         true,
	"00000000000000000000000000000000": true,
	"0000000000000000FFFF00000100007F": true,
}

// errListenerCheckUnsupported means that the host has no /proc/net/tcp to
// read.
var errListenerCheckUnsupported = xerrors.New("listener ownership check needs Linux")

// listenerLookup returns the owner uid of each socket that listens on port
// and accepts connections to 127.0.0.1.
type listenerLookup func(port int64) ([]int, error)

// loopbackListenerUIDs returns the owner uid of each socket in the LISTEN
// state on port that accepts connections to 127.0.0.1. It parses the text of
// /proc/net/tcp or /proc/net/tcp6.
func loopbackListenerUIDs(procNet string, port int64) []int {
	var uids []int
	for _, line := range strings.Split(procNet, "\n") {
		// Columns: sl local_address rem_address st tx_queue:rx_queue
		// tr:tm->when retrnsmt uid timeout inode. The header line fails
		// the state check.
		fields := strings.Fields(line)
		if len(fields) < 8 || fields[3] != procNetTCPListen {
			continue
		}
		addr, portHex, ok := strings.Cut(fields[1], ":")
		if !ok || !loopbackListenAddrs[strings.ToUpper(addr)] {
			continue
		}
		if p, err := strconv.ParseInt(portHex, 16, 32); err != nil || p != port {
			continue
		}
		uid, err := strconv.Atoi(fields[7])
		if err != nil {
			continue
		}
		uids = append(uids, uid)
	}
	return uids
}

// procListenerUIDs reads /proc/net/tcp and /proc/net/tcp6 for the listeners
// on port. It returns errListenerCheckUnsupported on a host other than
// Linux.
func procListenerUIDs(port int64) ([]int, error) {
	if runtime.GOOS != "linux" {
		return nil, errListenerCheckUnsupported
	}
	var uids []int
	for _, name := range []string{procNetTCP, procNetTCP6} {
		b, err := os.ReadFile(name)
		// A host with IPv6 off has no tcp6 file.
		if name == procNetTCP6 && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, xerrors.Errorf("read %s: %w", name, err)
		}
		uids = append(uids, loopbackListenerUIDs(string(b), port)...)
	}
	return uids, nil
}

// checkAPIListener refuses to continue when a socket that listens on the API
// port belongs to another OS user. Setup sends the admin password and the
// saved session token to that port, and they must reach only the server that
// develop started.
//
// When lookup finds no listener, or cannot read the socket table, it logs a
// warning and continues. On a host other than Linux it skips the check.
func checkAPIListener(ctx context.Context, logger slog.Logger, port int64, lookup listenerLookup) error {
	uids, err := lookup(port)
	switch {
	case errors.Is(err, errListenerCheckUnsupported):
		logger.Debug(ctx, "skipping API listener ownership check",
			slog.F("goos", runtime.GOOS), slog.F("api_port", port))
		return nil
	case err != nil:
		logger.Warn(ctx, "cannot check who owns the API listener, continuing",
			slog.F("api_port", port), slog.Error(err))
		return nil
	case len(uids) == 0:
		logger.Warn(ctx, "found no listener on the API port to check, continuing",
			slog.F("api_port", port))
		return nil
	}
	self := os.Getuid()
	for _, uid := range uids {
		if uid != self {
			return xerrors.Errorf("the listener on API port %d belongs to uid %d, not to uid %d that runs develop, so setup will not send credentials to it", port, uid, self)
		}
	}
	return nil
}
