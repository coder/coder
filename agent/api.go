package agent

import (
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

func (a *agent) apiHandler() (http.Handler, func() error) {
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

	cacheDuration := 1 * time.Second
	if a.portCacheDuration > 0 {
		cacheDuration = a.portCacheDuration
	}

	lp := &listeningPortsHandler{
		ignorePorts:   cpy,
		cacheDuration: cacheDuration,
	}

	if a.experimentalDevcontainersEnabled {
		containerAPIOpts := []agentcontainers.Option{
			agentcontainers.WithExecer(a.execer),
		}
		manifest := a.manifest.Load()
		if manifest != nil && len(manifest.Devcontainers) > 0 {
			containerAPIOpts = append(
				containerAPIOpts,
				agentcontainers.WithDevcontainers(manifest.Devcontainers),
			)
		}

		// Append after to allow the agent options to override the default options.
		containerAPIOpts = append(containerAPIOpts, a.containerAPIOptions...)

		containerAPI := agentcontainers.NewAPI(a.logger.Named("containers"), containerAPIOpts...)
		r.Mount("/api/v0/containers", containerAPI.Routes())
		a.containerAPI.Store(containerAPI)
	} else {
		r.HandleFunc("/api/v0/containers", func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(r.Context(), w, http.StatusForbidden, codersdk.Response{
				Message: "The agent dev containers feature is experimental and not enabled by default.",
				Detail:  "To enable this feature, set CODER_AGENT_DEVCONTAINERS_ENABLE=true in your template.",
			})
		})
	}

	promHandler := PrometheusMetricsHandler(a.prometheusRegistry, a.logger)

	r.Get("/api/v0/listening-ports", lp.handler)
	r.Get("/api/v0/netcheck", a.HandleNetcheck)
	r.Post("/api/v0/list-directory", a.HandleLS)
	r.Get("/debug/logs", a.HandleHTTPDebugLogs)
	r.Get("/debug/magicsock", a.HandleHTTPDebugMagicsock)
	r.Get("/debug/magicsock/debug-logging/{state}", a.HandleHTTPMagicsockDebugLoggingState)
	r.Get("/debug/manifest", a.HandleHTTPDebugManifest)
	r.Get("/debug/prometheus", promHandler.ServeHTTP)

	return r, func() error {
		if containerAPI := a.containerAPI.Load(); containerAPI != nil {
			return containerAPI.Close()
		}
		return nil
	}
}

type listeningPortsHandler struct {
	ignorePorts   map[int]string
	cacheDuration time.Duration

	//nolint: unused  // used on some but not all platforms
	mut sync.Mutex
	//nolint: unused  // used on some but not all platforms
	ports []codersdk.WorkspaceAgentListeningPort
	//nolint: unused  // used on some but not all platforms
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
