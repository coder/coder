package agent

import (
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

func (a *agent) apiHandler() http.Handler {
	r := chi.NewRouter()
	r.Get("/", func(rw http.ResponseWriter, r *http.Request) {
		httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
			Message: "Hello from the agent!",
		})
	})

	// Make a copy to ensure the map is not modified after the handler is
	// created.
	cpy := make(map[int]string)
	for k, b := range a.ignorePorts {
		cpy[k] = b
	}

	lp := &listeningPortsHandler{ignorePorts: cpy}
	r.Get("/api/v0/listening-ports", lp.handler)

	return r
}

type listeningPortsHandler struct {
	mut         sync.Mutex
	ports       []codersdk.WorkspaceAgentListeningPort
	mtime       time.Time
	ignorePorts map[int]string
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

	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.WorkspaceAgentListeningPortsResponse{
		Ports: ports,
	})
}
