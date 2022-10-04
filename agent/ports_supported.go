//go:build linux || windows
// +build linux windows

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

	ports := []codersdk.ListeningPort{}
	for _, tab := range tabs {
		if tab.LocalAddr.Port < uint16(codersdk.MinimumListeningPort) {
			continue
		}

		ports = append(ports, codersdk.ListeningPort{
			ProcessName: tab.Process.Name,
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
