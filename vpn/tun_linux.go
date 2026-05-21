//go:build linux

package vpn

import (
	"golang.org/x/xerrors"
	"tailscale.com/net/dns"
	"tailscale.com/net/netmon"
	"tailscale.com/net/tstun"
	"tailscale.com/wgengine/router"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/tailnet"
)

const defaultTunName = "coder0"

func GetNetworkingStack(_ *Tunnel, _ *StartRequest, logger slog.Logger) (NetworkStack, error) {
	tunDev, tunName, err := tstun.New(tailnet.Logger(logger.Named("net.tun.device")), defaultTunName)
	if err != nil {
		return NetworkStack{}, xerrors.Errorf("create tun device: %w", err)
	}

	wireguardMonitor, err := netmon.New(tailnet.Logger(logger.Named("net.wgmonitor")))
	if err != nil {
		return NetworkStack{}, xerrors.Errorf("create wireguard monitor: %w", err)
	}

	coderRouter, err := router.New(tailnet.Logger(logger.Named("net.router")), tunDev, wireguardMonitor)
	if err != nil {
		return NetworkStack{}, xerrors.Errorf("create router: %w", err)
	}

	dnsConfigurator, err := dns.NewOSConfigurator(tailnet.Logger(logger.Named("net.dns")), tunName)
	if err != nil {
		return NetworkStack{}, xerrors.Errorf("create dns configurator: %w", err)
	}

	return NetworkStack{
		WireguardMonitor: wireguardMonitor,
		TUNDevice:        tunDev,
		Router:           coderRouter,
		DNSConfigurator:  dnsConfigurator,
	}, nil
}
