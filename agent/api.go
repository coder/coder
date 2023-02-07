package agent

import (
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

func (*agent) apiHandler() http.Handler {
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
	ports []codersdk.WorkspaceAgentListeningPort
	mtime time.Time
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
