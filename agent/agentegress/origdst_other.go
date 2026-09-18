//go:build !linux

package agentegress

import (
	"net"
	"net/netip"
)

// originalDestination is only supported on Linux. Elsewhere every
// connection is treated as an explicit proxy client.
func originalDestination(net.Conn) (netip.AddrPort, error) {
	return netip.AddrPort{}, nil
}

// enableUDPOriginalDst is a no-op outside Linux, where nothing redirects
// UDP into the proxy.
func enableUDPOriginalDst(*net.UDPConn) error {
	return nil
}

// udpOriginalDst always reports "not redirected" outside Linux.
func udpOriginalDst([]byte) netip.AddrPort {
	return netip.AddrPort{}
}
