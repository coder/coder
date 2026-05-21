//go:build linux || (windows && amd64)

package agent

import (
	"sync"
	"time"

	"github.com/cakturk/go-netstat/netstat"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

type osListeningPortsGetter struct {
	cacheDuration time.Duration
	mut           sync.Mutex
	ports         []codersdk.WorkspaceAgentListeningPort
	mtime         time.Time
}

func (lp *osListeningPortsGetter) GetListeningPorts() ([]codersdk.WorkspaceAgentListeningPort, error) {
	lp.mut.Lock()
	defer lp.mut.Unlock()

	if time.Since(lp.mtime) < lp.cacheDuration {
		// copy
		ports := make([]codersdk.WorkspaceAgentListeningPort, len(lp.ports))
		copy(ports, lp.ports)
		return ports, nil
	}

	tabs, err := netstat.TCPSocks(func(s *netstat.SockTabEntry) bool {
		return s.State == netstat.Listen
	})
	if err != nil {
		return nil, xerrors.Errorf("scan listening ports: %w", err)
	}

	seen := make(map[uint16]struct{}, len(tabs))
	ports := []codersdk.WorkspaceAgentListeningPort{}
	for _, tab := range tabs {
		if tab.LocalAddr == nil {
			continue
		}

		// Don't include ports that we've already seen. This can happen on
		// Windows, and maybe on Linux if you're using a shared listener socket.
		if _, ok := seen[tab.LocalAddr.Port]; ok {
			continue
		}
		seen[tab.LocalAddr.Port] = struct{}{}

		procName := ""
		if tab.Process != nil {
			procName = tab.Process.Name
		}
		ports = append(ports, codersdk.WorkspaceAgentListeningPort{
			ProcessName: procName,
			Network:     "tcp",
			Port:        tab.LocalAddr.Port,
		})
	}

	lp.ports = ports
	lp.mtime = time.Now()

	// copy
	ports = make([]codersdk.WorkspaceAgentListeningPort, len(lp.ports))
	copy(ports, lp.ports)
	return ports, nil
}
