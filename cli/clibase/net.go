package clibase

import (
	"net"
	"strconv"

	"github.com/pion/udp"
	"golang.org/x/xerrors"
)

// Net abstracts CLI commands interacting with the operating system networking.
//
// At present, it covers opening local listening sockets, since doing this
// in testing is a challenge without flakes, since it's hard to pick a port we
// know a priori will be free.
type Net interface {
	// Listen has the same semantics as `net.Listen` but also supports `udp`
	Listen(network, address string) (net.Listener, error)
}

// osNet is an implementation that call the real OS for networking.
type osNet struct{}

func (osNet) Listen(network, address string) (net.Listener, error) {
	switch network {
	case "tcp", "tcp4", "tcp6", "unix", "unixpacket":
		return net.Listen(network, address)
	case "udp":
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, xerrors.Errorf("split %q: %w", address, err)
		}

		var portInt int
		portInt, err = strconv.Atoi(port)
		if err != nil {
			return nil, xerrors.Errorf("parse port %v from %q as int: %w", port, address, err)
		}

		// Use pion here so that we get a stream-style net.Conn listener, instead
		// of a packet-oriented connection that can read and write to multiple
		// addresses.
		return udp.Listen(network, &net.UDPAddr{
			IP:   net.ParseIP(host),
			Port: portInt,
		})
	default:
		return nil, xerrors.Errorf("unknown listen network %q", network)
	}
}
