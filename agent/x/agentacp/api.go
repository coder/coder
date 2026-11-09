package agentacp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// A request can initialize a harness subprocess and load its saved session
// before writing a prompt to stdin. Leave another 30 seconds for subprocess
// cleanup and sending the HTTP response.
const sessionRequestTimeout = sessionInitializationTimeout + stdinWriteTimeout + 30*time.Second

// Bound each event write so a stalled reader cannot hold the stream indefinitely.
const streamWriteTimeout = 30 * time.Second

// API serves the agent-local ACP interface. Callers authorize session access
// before dialing the agent.
type API struct {
	manager *Manager
}

// NewAPI constructs the agent-local HTTP and WebSocket API.
func NewAPI(m *Manager) *API {
	return &API{manager: m}
}

// WriteDeadlineMiddleware extends write deadlines for ACP session requests.
func (api *API) WriteDeadlineMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v0/acp/sessions" || strings.HasPrefix(r.URL.Path, "/api/v0/acp/sessions/") {
			controller := http.NewResponseController(w)
			deadline := time.Now().Add(sessionRequestTimeout)
			err := controller.SetWriteDeadline(deadline)
			if err != nil && !errors.Is(err, http.ErrNotSupported) {
				api.manager.logger.Warn(r.Context(), "extend ACP session response write deadline", slog.Error(err))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Routes returns handlers mounted under /api/v0/acp.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/harnesses", func(w http.ResponseWriter, r *http.Request) {
		catalog := api.manager.Catalog()
		api.write(w, r, catalog, nil)
	})
	r.Post("/sessions", func(w http.ResponseWriter, r *http.Request) {
		var req workspacesdk.ACPCreateSessionRequest
		if !readRequest(w, r, &req) {
			return
		}
		result, err := api.manager.Create(r.Context(), req)
		api.write(w, r, result, err)
	})
	r.Route("/sessions/session", func(r chi.Router) {
		r.Use(api.sessionIDMiddleware)
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

type sessionIDContextKey struct{}

func (api *API) sessionIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := parseSessionID(r)
		if id.SessionID == "" || id.HarnessSlug == "" || !filepath.IsAbs(id.WorkingDirectory) {
			api.write(w, r, nil, ErrInvalid)
			return
		}
		ctx := context.WithValue(r.Context(), sessionIDContextKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// sessionID returns the session identity validated by sessionIDMiddleware.
func sessionID(r *http.Request) workspacesdk.ACPSessionID {
	id, ok := r.Context().Value(sessionIDContextKey{}).(workspacesdk.ACPSessionID)
	if !ok {
		panic("developer error: session ID middleware not provided")
	}
	return id
}

// Native IDs and working directories are opaque values, so transport them as
// query parameters rather than embedding them in URL paths.
func parseSessionID(r *http.Request) workspacesdk.ACPSessionID {
	q := r.URL.Query()
	return workspacesdk.ACPSessionID{
		HarnessSlug:      q.Get("harness_slug"),
		WorkingDirectory: q.Get("working_directory"),
		SessionID:        q.Get("session_id"),
	}
}

func readRequest(w http.ResponseWriter, r *http.Request, result any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, httpapi.DefaultMaxRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	// Reject unknown options rather than silently ignoring requested behavior.
	// Workspace agents can outlive server upgrades, so new request fields break
	// compatibility with older agents when sent. Callers must omit unsupported
	// fields or require an agent update before using them.
	decoder.DisallowUnknownFields()
	err := decoder.Decode(result)
	if err == nil {
		var extra any
		err = decoder.Decode(&extra)
		if errors.Is(err, io.EOF) {
			return true
		}
	}

	status := http.StatusBadRequest
	response := codersdk.Response{Message: "Invalid ACP request body."}
	var sizeErr *http.MaxBytesError
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case err == nil:
		response.Detail = "Request body must contain exactly one JSON value."
	case errors.As(err, &sizeErr):
		httpapi.RecordRequestBodyLimit(r.Context(), sizeErr.Limit)
		status = http.StatusRequestEntityTooLarge
		response.Message = "Request body too large."
		response.Detail = fmt.Sprintf("Maximum request body size is %d bytes.", sizeErr.Limit)
	case errors.Is(err, io.EOF):
		response.Detail = "Request body must contain a JSON value."
	case errors.Is(err, io.ErrUnexpectedEOF):
		response.Detail = "Request body contains incomplete JSON."
	case errors.As(err, &syntaxErr):
		response.Detail = fmt.Sprintf("Request body must be valid JSON (byte offset %d).", syntaxErr.Offset)
	case errors.As(err, &typeErr):
		response.Detail = fmt.Sprintf("Request field %q must have type %s.", typeErr.Field, typeErr.Type)
		if typeErr.Field == "" {
			response.Detail = "Request body must be a JSON object."
		}
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		// encoding/json has no typed unknown-field error. Bound the field name
		// included in the response, without including its value.
		diagnostic := fmt.Sprintf("%.256s", err.Error())
		response.Detail = diagnostic + ". Check the field name. If this field is required, " +
			"update the workspace agent, typically by restarting the workspace with a current agent."
	default:
		response.Detail = "Could not read the request body. Retry the request."
	}
	httpapi.Write(r.Context(), w, status, response)
	return false
}

func readCursor(r *http.Request) (*workspacesdk.ACPCursor, error) {
	q := r.URL.Query()
	if !q.Has("generation_id") && !q.Has("after") {
		return nil, nil //nolint:nilnil // An absent cursor requests the full transcript.
	}
	generationID, err := uuid.Parse(q.Get("generation_id"))
	if err != nil || generationID == uuid.Nil {
		return nil, ErrInvalid
	}
	seq, err := strconv.ParseUint(q.Get("after"), 10, 64)
	if err != nil {
		return nil, ErrInvalid
	}
	return &workspacesdk.ACPCursor{GenerationID: generationID, Seq: seq}, nil
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
		case errors.Is(err, ErrUnavailable), errors.Is(err, ErrManagerClosed):
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
	watchCtx, cancelWatch := context.WithCancel(context.Background())
	defer cancelWatch()
	baseline, events, err := api.manager.Watch(watchCtx, sessionID(r), cursor)
	if err != nil {
		api.write(w, r, nil, err)
		return
	}
	controller := http.NewResponseController(w)
	err = controller.SetWriteDeadline(time.Time{})
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		api.manager.logger.Warn(watchCtx, "clear ACP stream response write deadline", slog.Error(err))
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() {
		if err := conn.CloseNow(); err != nil && !errors.Is(err, net.ErrClosed) {
			api.manager.logger.Debug(watchCtx, "close ACP stream websocket", slog.Error(err))
		}
	}()
	watchCtx = conn.CloseRead(watchCtx)
	write := func(event workspacesdk.ACPEvent) error {
		writeCtx, cancelWrite := timeout(watchCtx, api.manager.clock, streamWriteTimeout)
		defer cancelWrite()
		err := wsjson.Write(writeCtx, conn, event)
		if err != nil {
			log := api.manager.logger.Warn
			closeStatus := websocket.CloseStatus(err)
			if errors.Is(err, net.ErrClosed) || closeStatus == websocket.StatusNormalClosure || closeStatus == websocket.StatusGoingAway {
				log = api.manager.logger.Debug
			}
			log(watchCtx, "write ACP stream event",
				slog.Error(err),
				slog.F("event_kind", event.Kind),
				slog.F("cursor", event.Cursor),
			)
		}
		return err
	}
	for _, event := range baseline.Events {
		if write(event) != nil {
			return
		}
	}
	snapshot := workspacesdk.ACPEvent{
		Kind:    workspacesdk.ACPEventKindSnapshot,
		Cursor:  baseline.Session.Cursor,
		Session: &baseline.Session,
	}
	if write(snapshot) != nil {
		return
	}
	if sessionFinished(baseline.Session) {
		_ = conn.Close(websocket.StatusNormalClosure, "session completed")
		return
	}
	for {
		select {
		case <-watchCtx.Done():
			return
		case event, ok := <-events:
			if !ok {
				_ = conn.Close(websocket.StatusTryAgainLater, "reconnect with transcript cursor")
				return
			}
			if write(event) != nil {
				return
			}
			if event.Kind == workspacesdk.ACPEventKindStatus && event.Session != nil && sessionFinished(*event.Session) {
				_ = conn.Close(websocket.StatusNormalClosure, "session completed")
				return
			}
		}
	}
}

func sessionFinished(info workspacesdk.ACPSession) bool {
	return info.Status == workspacesdk.ACPSessionStatusIdle || info.Status == workspacesdk.ACPSessionStatusError
}
