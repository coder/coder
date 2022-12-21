//go:build linux || (windows && amd64)

package agent

import (
	"time"

	"github.com/cakturk/go-netstat/netstat"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

func (lp *listeningPortsHandler) getListeningPorts() ([]codersdk.ListeningPort, error) {
	lp.mut.Lock()
	defer lp.mut.Unlock()

	if time.Since(lp.mtime) < time.Second {
		// copy
		ports := make([]codersdk.ListeningPort, len(lp.ports))
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
	ports := []codersdk.ListeningPort{}
	for _, tab := range tabs {
		if tab.LocalAddr == nil || tab.LocalAddr.Port < codersdk.MinimumListeningPort {
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
		ports = append(ports, codersdk.ListeningPort{
			ProcessName: procName,
			Network:     codersdk.ListeningPortNetworkTCP,
			Port:        tab.LocalAddr.Port,
		})
	}

	lp.ports = ports
	lp.mtime = time.Now()

	// copy
	ports = make([]codersdk.ListeningPort, len(lp.ports))
	copy(ports, lp.ports)
	return ports, nil
}
