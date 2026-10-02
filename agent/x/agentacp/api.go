package agentacp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// API serves the agent-local ACP interface. Callers authorize session access
// before dialing the agent.
type API struct{ manager *Manager }

// NewAPI constructs the agent-local HTTP and WebSocket API.
func NewAPI(m *Manager) *API { return &API{manager: m} }

// Routes returns handlers mounted under /api/v0/acp.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// ACP setup can exceed the agent server's ordinary write timeout.
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(2 * setupTimeout))
			next.ServeHTTP(w, r)
		})
	})
	r.Get("/harnesses", func(w http.ResponseWriter, r *http.Request) { api.write(w, r, api.manager.Catalog(), nil) })
	r.Get("/sessions", func(w http.ResponseWriter, r *http.Request) { api.write(w, r, api.manager.List(), nil) })
	r.Post("/sessions", func(w http.ResponseWriter, r *http.Request) {
		var req workspacesdk.ACPCreateSessionRequest
		if !readRequest(w, r, &req) {
			return
		}
		result, err := api.manager.Create(r.Context(), req)
		api.write(w, r, result, err)
	})
	r.Route("/sessions/session", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if id := sessionID(r); id.SessionID == "" || id.HarnessSlug == "" || !filepath.IsAbs(id.WorkingDirectory) {
					api.write(w, r, nil, ErrInvalid)
					return
				}
				next.ServeHTTP(w, r)
			})
		})
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			cursor, err := readCursor(r)
			if err != nil {
				api.write(w, r, nil, err)
				return
			}
			result, err := api.manager.Read(r.Context(), sessionID(r), cursor)
			api.write(w, r, result, err)
		})
		r.Post("/messages", func(w http.ResponseWriter, r *http.Request) {
			var req workspacesdk.ACPMessageRequest
			if !readRequest(w, r, &req) {
				return
			}
			result, err := api.manager.Message(r.Context(), sessionID(r), req)
			api.write(w, r, result, err)
		})
		r.Post("/interrupt", func(w http.ResponseWriter, r *http.Request) {
			result, err := api.manager.Interrupt(r.Context(), sessionID(r))
			api.write(w, r, result, err)
		})
		r.Get("/stream", api.stream)
	})
	return r
}

// Native IDs and working directories are opaque values, so transport them as
// query parameters rather than embedding them in URL paths.
func sessionID(r *http.Request) workspacesdk.ACPSessionID {
	q := r.URL.Query()
	return workspacesdk.ACPSessionID{
		HarnessSlug: q.Get("harness_slug"), WorkingDirectory: q.Get("working_directory"), SessionID: q.Get("session_id"),
	}
}

func readRequest(w http.ResponseWriter, r *http.Request, result any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(result)
	if err == nil {
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			err = ErrInvalid
		}
	}
	if err != nil {
		httpapi.Write(r.Context(), w, http.StatusBadRequest, codersdk.Response{Message: "Invalid ACP request body."})
		return false
	}
	return true
}

func readCursor(r *http.Request) (*workspacesdk.ACPCursor, error) {
	q := r.URL.Query()
	if !q.Has("epoch") && !q.Has("after") {
		return nil, nil //nolint:nilnil // An absent cursor requests the full transcript.
	}
	epoch, err := uuid.Parse(q.Get("epoch"))
	if err != nil || epoch == uuid.Nil {
		return nil, ErrInvalid
	}
	seq, err := strconv.ParseUint(q.Get("after"), 10, 64)
	if err != nil {
		return nil, ErrInvalid
	}
	return &workspacesdk.ACPCursor{Epoch: epoch, Seq: seq}, nil
}

func (*API) write(w http.ResponseWriter, r *http.Request, result any, err error) {
	status := http.StatusOK
	if err != nil {
		status = http.StatusBadGateway
		switch {
		case errors.Is(err, ErrInvalid):
			status = http.StatusBadRequest
		case errors.Is(err, ErrNotFound):
			status = http.StatusNotFound
		case errors.Is(err, ErrConflict):
			status = http.StatusConflict
		case errors.Is(err, ErrUnavailable):
			status = http.StatusServiceUnavailable
		}
		result = codersdk.Response{Message: "ACP request failed.", Detail: err.Error()}
	}
	httpapi.Write(r.Context(), w, status, result)
}

func (api *API) stream(w http.ResponseWriter, r *http.Request) {
	cursor, err := readCursor(r)
	if err != nil {
		api.write(w, r, nil, err)
		return
	}
	baseline, events, unsubscribe, err := api.manager.Subscribe(r.Context(), sessionID(r), cursor)
	if err != nil {
		api.write(w, r, nil, err)
		return
	}
	defer unsubscribe()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()
	ctx := conn.CloseRead(r.Context())
	write := func(event workspacesdk.ACPEvent) error {
		ctx, cancel := api.manager.timeout(ctx, setupTimeout)
		defer cancel()
		return wsjson.Write(ctx, conn, event)
	}
	for _, event := range baseline {
		if write(event) != nil {
			return
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				_ = conn.Close(websocket.StatusTryAgainLater, "reconnect with transcript cursor")
				return
			}
			if write(event) != nil {
				return
			}
		}
	}
}
