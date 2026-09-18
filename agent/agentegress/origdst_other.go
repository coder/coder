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
