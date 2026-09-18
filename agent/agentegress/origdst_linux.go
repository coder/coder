//go:build linux

package agentegress

import (
	"encoding/binary"
	"net"
	"net/netip"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

// ip6tSoOriginalDst is IP6T_SO_ORIGINAL_DST from linux/netfilter_ipv6/
// ip6_tables.h. x/sys does not export it; it shares the value of the IPv4
// option.
const ip6tSoOriginalDst = unix.SO_ORIGINAL_DST

// originalDestination asks conntrack for the pre-REDIRECT destination of a
// TCP connection. It returns an invalid AddrPort with a nil error when the
// kernel has no NAT record for the socket, which is the normal case for
// explicit proxy clients. This path needs netfilter and therefore cannot be
// exercised by unit tests; it is kept minimal on purpose.
func originalDestination(conn net.Conn) (netip.AddrPort, error) {
	sc, ok := conn.(syscall.Conn)
	if !ok {
		return netip.AddrPort{}, xerrors.Errorf("connection type %T has no raw socket", conn)
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return netip.AddrPort{}, xerrors.Errorf("raw conn: %w", err)
	}
	var (
		dst    netip.AddrPort
		optErr error
	)
	ctlErr := raw.Control(func(fd uintptr) {
		dst, optErr = getOriginalDst(int(fd), conn.LocalAddr())
	})
	if ctlErr != nil {
		return netip.AddrPort{}, xerrors.Errorf("control raw conn: %w", ctlErr)
	}
	if optErr != nil {
		// ENOENT means conntrack has no entry, so the socket was not NATed.
		if xerrors.Is(optErr, unix.ENOENT) || xerrors.Is(optErr, unix.ENOPROTOOPT) {
			return netip.AddrPort{}, nil
		}
		return netip.AddrPort{}, optErr
	}
	return dst, nil
}

func getOriginalDst(fd int, local net.Addr) (netip.AddrPort, error) {
	tcpAddr, ok := local.(*net.TCPAddr)
	if !ok {
		return netip.AddrPort{}, xerrors.Errorf("unexpected local address type %T", local)
	}
	if tcpAddr.AddrPort().Addr().Unmap().Is4() {
		// sockaddr_in fits in the 16-byte multicast address of an
		// IPv6Mreq: family(2) port(2) addr(4) zero(8).
		mreq, err := unix.GetsockoptIPv6Mreq(fd, unix.IPPROTO_IP, unix.SO_ORIGINAL_DST)
		if err != nil {
			return netip.AddrPort{}, xerrors.Errorf("getsockopt SO_ORIGINAL_DST: %w", err)
		}
		port := binary.BigEndian.Uint16(mreq.Multiaddr[2:4])
		addr := netip.AddrFrom4([4]byte(mreq.Multiaddr[4:8]))
		return netip.AddrPortFrom(addr, port), nil
	}
	// sockaddr_in6 (28 bytes) fits in an IPv6MTUInfo (32 bytes).
	info, err := unix.GetsockoptIPv6MTUInfo(fd, unix.IPPROTO_IPV6, ip6tSoOriginalDst)
	if err != nil {
		return netip.AddrPort{}, xerrors.Errorf("getsockopt IP6T_SO_ORIGINAL_DST: %w", err)
	}
	// RawSockaddrInet6.Port holds the network-order bytes as a native
	// uint16, so re-read them in big endian.
	var portBytes [2]byte
	binary.NativeEndian.PutUint16(portBytes[:], info.Addr.Port)
	port := binary.BigEndian.Uint16(portBytes[:])
	addr := netip.AddrFrom16(info.Addr.Addr)
	return netip.AddrPortFrom(addr.Unmap(), port), nil
}
