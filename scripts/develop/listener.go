//go:build !windows

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
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

// decodeProcAddr decodes the local address of a row of /proc/net/tcp (8 hex
// digits, IPv4) or /proc/net/tcp6 (32 hex digits, IPv6). The kernel prints
// each 32-bit word of the address in host byte order.
func decodeProcAddr(s string) (net.IP, bool) {
	if len(s) != 2*net.IPv4len && len(s) != 2*net.IPv6len {
		return nil, false
	}
	ip := make(net.IP, len(s)/2)
	for i := 0; i < len(s); i += 8 {
		w, err := strconv.ParseUint(s[i:i+8], 16, 32)
		if err != nil {
			return nil, false
		}
		binary.NativeEndian.PutUint32(ip[i/2:], uint32(w))
	}
	return ip, true
}

// classifyListenAddr reports whether a socket that listens on ip can accept
// a connection to 127.0.0.1. That holds for 127.0.0.1 and 0.0.0.0 (ipv4), for
// :: (anyIPv6, when the socket is dual-stack) and for the IPv4-mapped forms
// of 127.0.0.1 and 0.0.0.0, ::ffff:127.0.0.1 and ::ffff:0.0.0.0.
func classifyListenAddr(ip net.IP) (ipv4, anyIPv6, ok bool) {
	acceptsLoopback := func(v4 net.IP) bool {
		return v4.Equal(net.IPv4zero) || v4.Equal(net.IPv4(127, 0, 0, 1))
	}
	switch len(ip) {
	case net.IPv4len:
		return true, false, acceptsLoopback(ip)
	case net.IPv6len:
		if ip.Equal(net.IPv6unspecified) {
			return false, true, true
		}
		if v4 := ip.To4(); v4 != nil {
			return false, false, acceptsLoopback(v4)
		}
	}
	return false, false, false
}

// errListenerCheckUnsupported means that the host has no /proc/net/tcp to
// read.
var errListenerCheckUnsupported = xerrors.New("listener ownership check needs Linux")

// listenerLookup returns the owner uid of each socket that listens on port
// and accepts connections to 127.0.0.1.
type listenerLookup func(port int64) ([]int, error)

// loopbackListener is a socket in the LISTEN state that can accept
// connections to 127.0.0.1.
type loopbackListener struct {
	uid int
	// ipv4 is true for a socket on 127.0.0.1 or 0.0.0.0. A socket on an
	// IPv4-mapped address, such as ::ffff:0.0.0.0, is neither ipv4 nor
	// anyIPv6.
	ipv4 bool
	// anyIPv6 is true for a socket on ::. The socket table does not show
	// its IPV6_V6ONLY option, so it may not accept connections to 127.0.0.1.
	anyIPv6 bool
}

// loopbackListeners returns each socket in the LISTEN state on port that can
// accept connections to 127.0.0.1. It parses the text of /proc/net/tcp or
// /proc/net/tcp6.
func loopbackListeners(procNet string, port int64) []loopbackListener {
	var listeners []loopbackListener
	for _, line := range strings.Split(procNet, "\n") {
		// Columns: sl local_address rem_address st tx_queue:rx_queue
		// tr:tm->when retrnsmt uid timeout inode. The header line fails
		// the state check.
		fields := strings.Fields(line)
		if len(fields) < 8 || fields[3] != procNetTCPListen {
			continue
		}
		addr, portHex, ok := strings.Cut(fields[1], ":")
		if !ok {
			continue
		}
		ip, ok := decodeProcAddr(addr)
		if !ok {
			continue
		}
		ipv4, anyIPv6, ok := classifyListenAddr(ip)
		if !ok {
			continue
		}
		if p, err := strconv.ParseInt(portHex, 16, 32); err != nil || p != port {
			continue
		}
		uid, err := strconv.Atoi(fields[7])
		if err != nil {
			continue
		}
		listeners = append(listeners, loopbackListener{uid: uid, ipv4: ipv4, anyIPv6: anyIPv6})
	}
	return listeners
}

// tableListenerUIDs returns the owner uid of each socket on port that the
// ownership check must consider, from the text of /proc/net/tcp and
// /proc/net/tcp6. self is the uid that runs develop.
//
// A socket on :: counts only when it can take a connection to 127.0.0.1.
// When a socket of uid self listens on 127.0.0.1 or 0.0.0.0 on the port, any
// socket on :: must be IPv6-only. The kernel refuses a dual-stack socket on a
// port that an IPv4 socket listens on, and requests to 127.0.0.1 go to the
// IPv4 socket. Sockets on any other address always count, including a
// socket on an IPv4-mapped address such as ::ffff:0.0.0.0, which takes IPv4
// connections whatever its IPV6_V6ONLY option says.
func tableListenerUIDs(tcp, tcp6 string, port int64, self int) []int {
	listeners := append(loopbackListeners(tcp, port), loopbackListeners(tcp6, port)...)
	ownIPv4 := false
	for _, l := range listeners {
		if l.uid == self && l.ipv4 {
			ownIPv4 = true
		}
	}
	var uids []int
	for _, l := range listeners {
		if ownIPv4 && l.anyIPv6 {
			continue
		}
		uids = append(uids, l.uid)
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
	tcp, err := os.ReadFile(procNetTCP)
	if err != nil {
		return nil, xerrors.Errorf("read %s: %w", procNetTCP, err)
	}
	tcp6, err := os.ReadFile(procNetTCP6)
	// A host with IPv6 off has no tcp6 file.
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, xerrors.Errorf("read %s: %w", procNetTCP6, err)
	}
	return tableListenerUIDs(string(tcp), string(tcp6), port, os.Getuid()), nil
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
