//go:build darwin

package vpn

import (
	"os"

	"github.com/tailscale/wireguard-go/tun"
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

func GetNetworkingStack(t *Tunnel, req *StartRequest, _ slog.Logger) (NetworkStack, error) {
	tunFd := int(req.GetTunnelFileDescriptor())
	dupTunFd, err := unix.Dup(tunFd)
	if err != nil {
		return NetworkStack{}, xerrors.Errorf("dup tun fd: %w", err)
	}

	err = unix.SetNonblock(dupTunFd, true)
	if err != nil {
		unix.Close(dupTunFd)
		return NetworkStack{}, xerrors.Errorf("set nonblock: %w", err)
	}
	fileTun, err := tun.CreateTUNFromFile(os.NewFile(uintptr(dupTunFd), "/dev/tun"), 0)
	if err != nil {
		unix.Close(dupTunFd)
		return NetworkStack{}, xerrors.Errorf("create TUN from File: %w", err)
	}

	return NetworkStack{
		WireguardMonitor: nil, // default is fine
		TUNDevice:        fileTun,
		Router:           NewRouter(t),
		DNSConfigurator:  NewDNSConfigurator(t),
	}, nil
}
