//go:build linux

package agentssh

import (
	"fmt"
	"os"

	"github.com/cakturk/go-netstat/netstat"
	"golang.org/x/xerrors"
)

func getListeningPortProcessCmdline(port uint32) (string, error) {
	tabs, err := netstat.TCPSocks(func(s *netstat.SockTabEntry) bool {
		return s.LocalAddr != nil && uint32(s.LocalAddr.Port) == port
	})
	if err != nil {
		return "", xerrors.Errorf("inspect port %d: %w", port, err)
	}
	if len(tabs) == 0 {
		return "", nil
	}
	// The process name provided by go-netstat does not include the full command
	// line so grab that instead.
	pid := tabs[0].Process.Pid
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return "", xerrors.Errorf("read /proc/%d/cmdline: %w", pid, err)
	}
	return string(data), nil
}
