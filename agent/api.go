package agent

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/x/agentnative"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/httpmw"
	"github.com/coder/websocket"
)

func (a *agent) apiHandler(upgradeListener *httpUpgrader) http.Handler {
	r := chi.NewRouter()
	r.Use(
		httpmw.Recover(a.logger),
		tracing.StatusWriterMiddleware,
		// Reuse the coderd tracing middleware with a noop tracer (nil provider):
		// it emits no spans or telemetry on the agent and only enriches the log
		// context with client_session_id. Tracks the API, debug (for
		// support-bundle correlation), and root routes.
		tracing.Middleware(nil, []string{"/", "/api", "/api/**", "/debug/**"}, "agent"),
		loggermw.Logger(a.logger, nil),
		agentchat.Middleware,
		a.toolCalls.Middleware,
	)
	r.Get("/", func(rw http.ResponseWriter, r *http.Request) {
		httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
			Message: "Hello from the agent!",
		})
	})

	r.Mount("/api/v0", a.filesAPI.Routes())
	r.Mount("/api/v0/git", a.gitAPI.Routes())
	r.Mount("/api/v0/processes", a.processAPI.Routes())
	r.Mount("/api/v0/tool-calls", a.toolCalls.Routes())
	r.Mount("/api/v0/desktop", a.desktopAPI.Routes())
	r.Mount("/api/v0/mcp", a.mcpAPI.Routes())
	// The authenticated local manager API, including attach credentials, must
	// never be mounted on the remotely accessible agent API.
	r.Get("/api/v0/native-agents/acp", a.handleNativeAgentACP)
	r.Mount("/api/v0/context-config", a.contextConfigAPI.Routes())
	if a.contextAPI != nil {
		r.Mount("/api/v0/context", a.contextAPI.Routes())
	}

	if a.devcontainers {
		r.Mount("/api/v0/containers", a.containerAPI.Routes())
	} else if manifest := a.manifest.Load(); manifest != nil && manifest.ParentID != uuid.Nil {
		r.HandleFunc("/api/v0/containers", func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(r.Context(), w, http.StatusForbidden, codersdk.Response{
				Message: "Dev Container feature not supported.",
				Detail:  "Dev Container integration inside other Dev Containers is explicitly not supported.",
			})
		})
	} else {
		r.HandleFunc("/api/v0/containers", func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(r.Context(), w, http.StatusForbidden, codersdk.Response{
				Message: "Dev Container feature not enabled.",
				Detail:  "To enable this feature, set CODER_AGENT_DEVCONTAINERS_ENABLE=true in your template.",
			})
		})
	}

	promHandler := PrometheusMetricsHandler(a.prometheusRegistry, a.logger)

	r.Get("/api/v0/listening-ports", a.listeningPortsHandler.handler)
	r.Get("/api/v0/netcheck", a.HandleNetcheck)
	r.Get("/api/v0/tcp/{port}", upgradeListener.handler)
	r.Get("/debug/logs", a.HandleHTTPDebugLogs)
	r.Get("/debug/magicsock", a.HandleHTTPDebugMagicsock)
	r.Get("/debug/magicsock/debug-logging/{state}", a.HandleHTTPMagicsockDebugLoggingState)
	r.Get("/debug/manifest", a.HandleHTTPDebugManifest)
	r.Get("/debug/prometheus", promHandler.ServeHTTP)

	return r
}

func (a *agent) handleNativeAgentACP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !a.nativeAgents {
		httpapi.Write(ctx, w, http.StatusNotFound, codersdk.Response{Message: "Native agents are disabled."})
		return
	}
	manager := a.nativeManager.Load()
	if manager == nil {
		httpapi.Write(ctx, w, http.StatusServiceUnavailable, codersdk.Response{Message: "Native agents are not ready."})
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer func() { _ = ws.CloseNow() }()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Agent shutdown detaches this transport without stopping native services.
	stop := context.AfterFunc(a.gracefulCtx, cancel)
	defer stop()
	ctx, transport := codersdk.WebsocketNetConn(ctx, ws, websocket.MessageBinary)
	// NetConn disables read limits, so set ours after wrapping.
	ws.SetReadLimit(4 << 20)
	defer transport.Close()
	if err := agentnative.ServeACP(ctx, transport, manager); err != nil && ctx.Err() == nil {
		a.logger.Debug(ctx, "native agents ACP transport closed")
		_ = ws.Close(websocket.StatusInternalError, "Native agents transport closed.")
	}
}

type ListeningPortsGetter interface {
	GetListeningPorts() ([]codersdk.WorkspaceAgentListeningPort, error)
}

type listeningPortsHandler struct {
	// In production code, this is set to an osListeningPortsGetter, but it can be overridden for
	// testing.
	getter      ListeningPortsGetter
	ignorePorts map[int]string
}

// handler returns a list of listening ports. This is tested by coderd's
// TestWorkspaceAgentListeningPorts test.
func (lp *listeningPortsHandler) handler(rw http.ResponseWriter, r *http.Request) {
	ports, err := lp.getter.GetListeningPorts()
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Could not scan for listening ports.",
			Detail:  err.Error(),
		})
		return
	}

	filteredPorts := make([]codersdk.WorkspaceAgentListeningPort, 0, len(ports))
	for _, port := range ports {
		if port.Port < workspacesdk.AgentMinimumListeningPort {
			continue
		}

		// Ignore ports that we've been told to ignore.
		if _, ok := lp.ignorePorts[int(port.Port)]; ok {
			continue
		}
		filteredPorts = append(filteredPorts, port)
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.WorkspaceAgentListeningPortsResponse{
		Ports: filteredPorts,
	})
}
