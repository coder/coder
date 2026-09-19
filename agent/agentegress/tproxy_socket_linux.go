//go:build linux

package agentegress

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

var transparentUDPPorts sync.Map

func configureUDPOriginalDst(conn *net.UDPConn) (bool, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return false, xerrors.Errorf("raw conn: %w", err)
	}
	var (
		transparent bool
		optErr      error
	)
	if err := raw.Control(func(fd uintptr) {
		if err := unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_TRANSPARENT, 1); err == nil {
			transparent = true
		}
		optErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_RECVORIGDSTADDR, 1)
	}); err != nil {
		return false, xerrors.Errorf("control raw conn: %w", err)
	}
	if optErr != nil {
		return false, xerrors.Errorf("setsockopt IP_RECVORIGDSTADDR: %w", optErr)
	}
	return transparent, nil
}

func registerTransparentUDP(port uint16) {
	transparentUDPPorts.Store(port, struct{}{})
}

func unregisterTransparentUDP(port uint16) {
	transparentUDPPorts.Delete(port)
}

func udpTransparentReady(port uint16) bool {
	_, ok := transparentUDPPorts.Load(port)
	return ok
}

func transparentUDPReplyConn(dst netip.AddrPort) (*net.UDPConn, error) {
	var lc net.ListenConfig
	lc.Control = func(_, _ string, raw syscall.RawConn) error {
		var optErr error
		if err := raw.Control(func(fd uintptr) {
			if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
				optErr = err
				return
			}
			if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_MARK, 0x4350); err != nil {
				optErr = err
				return
			}
			optErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_TRANSPARENT, 1)
		}); err != nil {
			return xerrors.Errorf("control transparent reply socket: %w", err)
		}
		if optErr != nil {
			return xerrors.Errorf("configure transparent reply socket: %w", optErr)
		}
		return nil
	}
	pc, err := lc.ListenPacket(context.Background(), "udp4", dst.String())
	if err != nil {
		return nil, xerrors.Errorf("bind transparent reply socket to %s: %w", dst, err)
	}
	conn, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		return nil, xerrors.Errorf("unexpected transparent reply conn type %T", pc)
	}
	return conn, nil
}
