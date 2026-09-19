//go:build !linux

package agentegress

import (
	"errors"
	"net"
	"net/netip"
)

func configureUDPOriginalDst(*net.UDPConn) (bool, error) { return false, nil }
func registerTransparentUDP(uint16)                      {}
func unregisterTransparentUDP(uint16)                    {}
func udpTransparentReady(uint16) bool                    { return false }
func transparentUDPReplyConn(netip.AddrPort) (*net.UDPConn, error) {
	return nil, errors.ErrUnsupported
}
