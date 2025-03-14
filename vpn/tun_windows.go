//go:build windows
package vpn
import (
	"fmt"
	"context"
	"errors"
	"time"
	"github.com/dblohm7/wingoes/com"
	"github.com/tailscale/wireguard-go/tun"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.zx2c4.com/wintun"
	"tailscale.com/net/dns"
	"tailscale.com/net/netmon"
	"tailscale.com/net/tstun"
	"tailscale.com/types/logger"
	"tailscale.com/util/winutil"
	"tailscale.com/wgengine/router"
	"cdr.dev/slog"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/retry"
)
const (
	tunName = "Coder"
	tunGUID = "{0ed1515d-04a4-4c46-abae-11ad07cf0e6d}"
	wintunDLL = "wintun.dll"
)
func GetNetworkingStack(t *Tunnel, _ *StartRequest, logger slog.Logger) (NetworkStack, error) {
	// Initialize COM process-wide so Tailscale can make calls to the windows
	// network APIs to read/write adapter state.
	comProcessType := com.ConsoleApp
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		return NetworkStack{}, fmt.Errorf("svc.IsWindowsService failed: %w", err)
	}
	if isSvc {
		comProcessType = com.Service
	}
	if err := com.StartRuntime(comProcessType); err != nil {
		return NetworkStack{}, fmt.Errorf("could not initialize COM: com.StartRuntime(%d): %w", comProcessType, err)
	}
	// Set the name and GUID for the TUN interface.
	tun.WintunTunnelType = tunName
	guid, err := windows.GUIDFromString(tunGUID)
	if err != nil {
		return NetworkStack{}, fmt.Errorf("could not parse GUID %q: %w", tunGUID, err)
	}
	tun.WintunStaticRequestedGUID = &guid
	// Ensure wintun.dll is available, and fail early if it's not to avoid
	// hanging for 5 minutes in tstunNewWithWindowsRetries.
	//
	// First, we call wintun.Version() to make the wintun package attempt to
	// load wintun.dll. This allows the wintun package to set the logging
	// callback in the DLL before we load it ourselves.
	_ = wintun.Version()
	// Then, we try to load wintun.dll ourselves so we get a better error
	// message if there was a problem. This call matches the wintun package, so
	// we're loading it in the same way.
	//
	// Note: this leaks the handle to wintun.dll, but since it's already loaded
	// it wouldn't be freed anyways.
	const (
		LOAD_LIBRARY_SEARCH_APPLICATION_DIR = 0x00000200
		LOAD_LIBRARY_SEARCH_SYSTEM32        = 0x00000800
	)
	_, err = windows.LoadLibraryEx(wintunDLL, 0, LOAD_LIBRARY_SEARCH_APPLICATION_DIR|LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return NetworkStack{}, fmt.Errorf("could not load %q, it should be in the same directory as the executable (in Coder Desktop, this should have been installed automatically): %w", wintunDLL, err)
	}
	tunDev, tunName, err := tstunNewWithWindowsRetries(tailnet.Logger(logger.Named("net.tun.device")), tunName)
	if err != nil {
		return NetworkStack{}, fmt.Errorf("create tun device: %w", err)
	}
	logger.Info(context.Background(), "tun created", slog.F("name", tunName))
	wireguardMonitor, err := netmon.New(tailnet.Logger(logger.Named("net.wgmonitor")))
	coderRouter, err := router.New(tailnet.Logger(logger.Named("net.router")), tunDev, wireguardMonitor)
	if err != nil {
		return NetworkStack{}, fmt.Errorf("create router: %w", err)
	}
	dnsConfigurator, err := dns.NewOSConfigurator(tailnet.Logger(logger.Named("net.dns")), tunName)
	if err != nil {
		return NetworkStack{}, fmt.Errorf("create dns configurator: %w", err)
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
