//go:build darwin
package vpn
import (
	"fmt"
	"errors"
	"os"
	"github.com/tailscale/wireguard-go/tun"
	"golang.org/x/sys/unix"
	"cdr.dev/slog"
)
func GetNetworkingStack(t *Tunnel, req *StartRequest, _ slog.Logger) (NetworkStack, error) {
	tunFd := int(req.GetTunnelFileDescriptor())
	dupTunFd, err := unix.Dup(tunFd)
	if err != nil {
		return NetworkStack{}, fmt.Errorf("dup tun fd: %w", err)
	}
	err = unix.SetNonblock(dupTunFd, true)
	if err != nil {
		unix.Close(dupTunFd)
		return NetworkStack{}, fmt.Errorf("set nonblock: %w", err)
	}
	fileTun, err := tun.CreateTUNFromFile(os.NewFile(uintptr(dupTunFd), "/dev/tun"), 0)
	if err != nil {
		unix.Close(dupTunFd)
		return NetworkStack{}, fmt.Errorf("create TUN from File: %w", err)
	}
	return NetworkStack{
		WireguardMonitor: nil, // default is fine
		TUNDevice:        fileTun,
		Router:           NewRouter(t),
		DNSConfigurator:  NewDNSConfigurator(t),
	}, nil
}
