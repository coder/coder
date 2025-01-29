//go:build windows

package vpn

import (
	"context"
	"errors"
	"time"

	"github.com/coder/retry"
	"github.com/tailscale/wireguard-go/tun"
	"golang.org/x/sys/windows"
	"golang.org/x/xerrors"
	"golang.zx2c4.com/wintun"
	"tailscale.com/net/dns"
	"tailscale.com/net/netmon"
	"tailscale.com/net/tstun"
	"tailscale.com/types/logger"
	"tailscale.com/util/winutil"
	"tailscale.com/wgengine/router"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/tailnet"
)

const tunName = "Coder"

func GetNetworkingStack(t *Tunnel, _ *StartRequest, logger slog.Logger) (NetworkStack, error) {
	tun.WintunTunnelType = tunName
	guid, err := windows.GUIDFromString("{0ed1515d-04a4-4c46-abae-11ad07cf0e6d}")
	if err != nil {
		panic(err)
	}
	tun.WintunStaticRequestedGUID = &guid

	tunDev, tunName, err := tstunNewWithWindowsRetries(tailnet.Logger(logger.Named("net.tun.device")), tunName)
	if err != nil {
		return NetworkStack{}, xerrors.Errorf("create tun device: %w", err)
	}
	logger.Info(context.Background(), "tun created", slog.F("name", tunName))

	wireguardMonitor, err := netmon.New(tailnet.Logger(logger.Named("net.wgmonitor")))

	coderRouter, err := router.New(tailnet.Logger(logger.Named("net.router")), tunDev, wireguardMonitor)
	if err != nil {
		return NetworkStack{}, xerrors.Errorf("create router: %w", err)
	}

	dnsConfigurator, err := dns.NewOSConfigurator(tailnet.Logger(logger.Named("net.dns")), tunName)
	if err != nil {
		return NetworkStack{}, xerrors.Errorf("create dns configurator: %w", err)
	}

	return NetworkStack{
		WireguardMonitor: nil, // default is fine
		TUNDevice:        tunDev,
		Router:           coderRouter,
		DNSConfigurator:  dnsConfigurator,
	}, nil
}

// tstunNewOrRetry is a wrapper around tstun.New that retries on Windows for certain
// errors.
//
// This is taken from Tailscale:
// https://github.com/tailscale/tailscale/blob/3abfbf50aebbe3ba57dc749165edb56be6715c0a/cmd/tailscaled/tailscaled_windows.go#L107
func tstunNewWithWindowsRetries(logf logger.Logf, tunName string) (_ tun.Device, devName string, _ error) {
	r := retry.New(250*time.Millisecond, 10*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	for r.Wait(ctx) {
		dev, devName, err := tstun.New(logf, tunName)
		if err == nil {
			return dev, devName, err
		}
		if errors.Is(err, windows.ERROR_DEVICE_NOT_AVAILABLE) || windowsUptime() < 10*time.Minute {
			// Wintun is not installing correctly. Dump the state of NetSetupSvc
			// (which is a user-mode service that must be active for network devices
			// to install) and its dependencies to the log.
			winutil.LogSvcState(logf, "NetSetupSvc")
		}
	}

	return nil, "", ctx.Err()
}

var (
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	getTickCount64Proc = kernel32.NewProc("GetTickCount64")
)

func windowsUptime() time.Duration {
	r, _, _ := getTickCount64Proc.Call()
	return time.Duration(int64(r)) * time.Millisecond
}

// TODO(@dean): implement a way to install/uninstall the wintun driver, most
// likely as a CLI command
//
// This is taken from Tailscale:
// https://github.com/tailscale/tailscale/blob/3abfbf50aebbe3ba57dc749165edb56be6715c0a/cmd/tailscaled/tailscaled_windows.go#L543
func uninstallWinTun(logf logger.Logf) {
	dll := windows.NewLazyDLL("wintun.dll")
	if err := dll.Load(); err != nil {
		logf("Cannot load wintun.dll for uninstall: %v", err)
		return
	}

	logf("Removing wintun driver...")
	err := wintun.Uninstall()
	logf("Uninstall: %v", err)
}

// TODO(@dean): remove
var _ = uninstallWinTun
