//go:build linux

package agentssh

import (
	"errors"
	"fmt"
	"os"

	"github.com/cakturk/go-netstat/netstat"
	"golang.org/x/xerrors"
)

func getListeningPortProcessCmdline(port uint32) (string, error) {
	acceptFn := func(s *netstat.SockTabEntry) bool {
		return s.LocalAddr != nil && uint32(s.LocalAddr.Port) == port
	}
	tabs, err := netstat.TCPSocks(acceptFn)
	tabs6, err6 := netstat.TCP6Socks(acceptFn)

	// Only return the error if the other method found nothing.
	if (err != nil && len(tabs6) == 0) || (err6 != nil && len(tabs) == 0) {
		return "", xerrors.Errorf("inspect port %d: %w", port, errors.Join(err, err6))
	}

	var proc *netstat.Process
	if len(tabs) > 0 {
		proc = tabs[0].Process
	} else if len(tabs6) > 0 {
		proc = tabs6[0].Process
	}
	if proc == nil {
		// Either nothing is listening on this port or we were unable to read the
		// process details (permission issues reading /proc/$pid/* potentially).
		// Or, perhaps /proc/net/tcp{,6} is not listing the port for some reason.
		return "", nil
	}

	// The process name provided by go-netstat does not include the full command
	// line so grab that instead.
	pid := proc.Pid
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return "", xerrors.Errorf("read /proc/%d/cmdline: %w", pid, err)
	}
	return string(data), nil
}
