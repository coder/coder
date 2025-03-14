//go:build linux

package agentssh
import (

	"errors"
	"fmt"
	"os"
	"github.com/cakturk/go-netstat/netstat"
)

func getListeningPortProcessCmdline(port uint32) (string, error) {
	acceptFn := func(s *netstat.SockTabEntry) bool {
		return s.LocalAddr != nil && uint32(s.LocalAddr.Port) == port
	}

	tabs4, err4 := netstat.TCPSocks(acceptFn)
	tabs6, err6 := netstat.TCP6Socks(acceptFn)
	// In the common case, we want to check ipv4 listening addresses.  If this
	// fails, we should return an error.  We also need to check ipv6.  The
	// assumption is, if we have an err4, and 0 ipv6 addresses listed, then we are
	// interested in the err4 (and vice versa).  So return both errors (at least 1
	// is non-nil) if the other list is empty.

	if (err4 != nil && len(tabs6) == 0) || (err6 != nil && len(tabs4) == 0) {
		return "", fmt.Errorf("inspect port %d: %w", port, errors.Join(err4, err6))
	}
	var proc *netstat.Process
	if len(tabs4) > 0 {
		proc = tabs4[0].Process
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
		return "", fmt.Errorf("read /proc/%d/cmdline: %w", pid, err)
	}

	return string(data), nil
}
