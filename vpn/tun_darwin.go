//go:build darwin

package vpn

import (
	"os"

	"github.com/tailscale/wireguard-go/tun"
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

func makeTUN(tunFD int) (tun.Device, error) {
	dupTunFd, err := unix.Dup(tunFD)
	if err != nil {
		return nil, xerrors.Errorf("dup tun fd: %w", err)
	}

	err = unix.SetNonblock(dupTunFd, true)
	if err != nil {
		unix.Close(dupTunFd)
		return nil, xerrors.Errorf("set nonblock: %w", err)
	}
	fileTun, err := tun.CreateTUNFromFile(os.NewFile(uintptr(dupTunFd), "/dev/tun"), 0)
	if err != nil {
		unix.Close(dupTunFd)
		return nil, xerrors.Errorf("create TUN from File: %w", err)
	}
	return fileTun, nil
}
