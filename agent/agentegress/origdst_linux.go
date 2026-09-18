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

// enableUDPOriginalDst asks the kernel to attach the pre-REDIRECT
// destination of every datagram as an IP_RECVORIGDSTADDR control message.
func enableUDPOriginalDst(conn *net.UDPConn) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return xerrors.Errorf("raw conn: %w", err)
	}
	var optErr error
	if err := raw.Control(func(fd uintptr) {
		optErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_RECVORIGDSTADDR, 1)
	}); err != nil {
		return xerrors.Errorf("control raw conn: %w", err)
	}
	if optErr != nil {
		return xerrors.Errorf("setsockopt IP_RECVORIGDSTADDR: %w", optErr)
	}
	return nil
}

// udpOriginalDst extracts the original destination from the control
// messages returned by ReadMsgUDP. It returns an invalid AddrPort when the
// datagram was not redirected.
func udpOriginalDst(oob []byte) netip.AddrPort {
	msgs, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return netip.AddrPort{}
	}
	for _, m := range msgs {
		if m.Header.Level != unix.IPPROTO_IP || m.Header.Type != unix.IP_RECVORIGDSTADDR {
			continue
		}
		// sockaddr_in: family(2) port(2, network order) addr(4) zero(8).
		if len(m.Data) < 8 || binary.NativeEndian.Uint16(m.Data[0:2]) != unix.AF_INET {
			continue
		}
		port := binary.BigEndian.Uint16(m.Data[2:4])
		addr := netip.AddrFrom4([4]byte(m.Data[4:8]))
		return netip.AddrPortFrom(addr, port)
	}
	return netip.AddrPort{}
}

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
