//go:build linux

package agentssh

import (
	"errors"
	"fmt"
	"os"

	"github.com/cakturk/go-netstat/netstat"
	"golang.org/x/xerrors"
)

/**
 * getListeningPortProcessCmdlines looks up the full command line for all
 * processes listening on the specified port regardless of their address.  It
 * returns all the commands it was able to find and any errors encountered while
 * doing the search.
 */
func getListeningPortProcessCmdlines(port uint32) ([]string, error) {
	acceptFn := func(s *netstat.SockTabEntry) bool {
		return s.LocalAddr != nil && uint32(s.LocalAddr.Port) == port
	}

	tabs4, err4 := netstat.TCPSocks(acceptFn)
	tabs6, err6 := netstat.TCP6Socks(acceptFn)

	allErrs := errors.Join(err4, err6)

	var cmdlines []string
	for _, tab := range append(tabs4, tabs6...) {
		// If the process is nil then perhaps we were unable to read the process
		// details (permission issues reading /proc/$pid/* maybe).
		if tab.Process == nil {
			allErrs = errors.Join(allErrs, xerrors.Errorf("read process on port %d", port))
			continue
		}
		// The process name provided by go-netstat does not include the full command
		// line so grab that instead.
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", tab.Process.Pid))
		if err != nil {
			allErrs = errors.Join(allErrs, xerrors.Errorf("read /proc/%d/cmdline: %w", tab.Process.Pid, err))
			continue
		}
		cmdlines = append(cmdlines, string(data))
	}

	// Always send back as much as we found and the errors.  Note that if cmdlines
	// is empty then either nothing is listening on this port or perhaps there
	// were permission issues reading /proc/net/tcp{,6}.
	return cmdlines, allErrs
}
