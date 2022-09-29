package agent

import (
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/cakturk/go-netstat/netstat"
	"github.com/go-chi/chi"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

func (*agent) statisticsHandler() http.Handler {
	r := chi.NewRouter()
	r.Get("/", func(rw http.ResponseWriter, r *http.Request) {
		httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
			Message: "Hello from the agent!",
		})
	})

	lp := &listeningPortsHandler{}
	r.Get("/api/v0/listening-ports", lp.handler)

	return r
}

type listeningPortsHandler struct {
	mut   sync.Mutex
	ports []codersdk.ListeningPort
	mtime time.Time
}

func (lp *listeningPortsHandler) getListeningPorts() ([]codersdk.ListeningPort, error) {
	lp.mut.Lock()
	defer lp.mut.Unlock()

	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		// Can't scan for ports on non-linux or non-windows systems at the
		// moment. The UI will not show any "no ports found" message to the
		// user, so the user won't suspect a thing.
		return []codersdk.ListeningPort{}, nil
	}

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

// handler returns a list of listening ports. This is tested by coderd's
// TestWorkspaceAgentListeningPorts test.
func (lp *listeningPortsHandler) handler(rw http.ResponseWriter, r *http.Request) {
	ports, err := lp.getListeningPorts()
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Could not scan for listening ports.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.ListeningPortsResponse{
		Ports: ports,
	})
}
