package agenttoolcall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// Errors that answer a request without acting on the tool call.
var (
	errToolCallCanceled = xerrors.New("tool call was canceled before the agent received it")
	errToolCallUnknown  = xerrors.New("agent cannot tell whether the tool call ran")
	errRequestMismatch  = xerrors.New("request differs from the recorded request")
)

// ToolCall is the tool call a request runs for, as seen by the handler
// behind Middleware.
type ToolCall struct {
	Key Key
	// UUID is workspacesdk.ToolCallUUID of Key, the ID of anything the
	// tool call creates, such as a process.
	UUID uuid.UUID

	store     *Store
	rec       *record
	abandoned atomic.Bool
}

// OnCancel registers the cancel hook of the work the handler started.
// stop stops the work, and stopped closes once it has stopped. stop runs
// at most once: here, if the tool call is already canceled, or when a
// cancel arrives. A cancel answers after stopped closes. Only the first
// registration per tool call counts.
func (tc *ToolCall) OnCancel(stop func() error, stopped <-chan struct{}) {
	hook := &cancelHook{stop: stop, stopped: stopped, ran: make(chan struct{})}
	if tc.store.setHook(tc.rec, hook) {
		hook.run()
	}
}

// Abandon drops the tool call's record instead of recording the handler's
// response, so that the next request for the tool call runs the handler
// again. A handler calls it only when it fails before any side effect
// because it could not read the request body. The request that ran the
// handler still gets the handler's response.
func (tc *ToolCall) Abandon() {
	tc.abandoned.Store(true)
}

type toolCallContextKey struct{}

// FromContext returns the tool call that Middleware put in the context of
// a handler it runs.
func FromContext(ctx context.Context) (*ToolCall, bool) {
	tc, ok := ctx.Value(toolCallContextKey{}).(*ToolCall)
	return tc, ok
}

// Middleware runs next at most once per tool call and records its
// response. A request without tool call headers goes to next unchanged. A
// request with them gets the recorded response of the tool call's one
// run, waiting for a run in progress, or a 409 when the agent must not
// run it. The method, URL path, and raw query must match the recorded
// request. The middleware never reads the request body, so a request
// that waited for an abandoned run can run the handler with its own body.
func (s *Store) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		key, present, badRequest := toolCallFromRequest(r)
		if !present {
			next.ServeHTTP(rw, r)
			return
		}
		if badRequest != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, *badRequest)
			return
		}

		for {
			rec, created, err := s.begin(key, r)
			if writeToolCallError(ctx, rw, err) {
				return
			}
			if created {
				s.run(rw, r, next, key, rec)
				return
			}
			if !waitDone(ctx, rec) {
				// The client is gone.
				return
			}
			switch rec.state {
			case stateRecorded:
				writeResponse(rw, rec.resp)
				return
			case stateCanceledMarker:
				writeToolCallError(ctx, rw, errToolCallCanceled)
				return
			case stateDropped:
				// The run was abandoned; start over, and run the
				// handler if no other request did.
			default:
				panic(fmt.Sprintf("developer error: tool call record in state %d after its run", rec.state))
			}
		}
	})
}

// run runs next for rec's tool call and writes its response to rw. The
// response is recorded unless the handler abandons the run. If next
// panics, the run records a 500 so that waiters and later requests do not
// wait forever, and the panic continues.
func (s *Store) run(rw http.ResponseWriter, r *http.Request, next http.Handler, key Key, rec *record) {
	tc := &ToolCall{
		Key:   key,
		UUID:  workspacesdk.ToolCallUUID(key.ChatID, key.MessageID, key.ToolName, key.ToolCallID),
		store: s,
		rec:   rec,
	}
	recorder := &responseRecorder{header: http.Header{}}
	returned := false
	defer func() {
		if !returned {
			s.finish(key, rec, panicResponse)
		}
	}()
	next.ServeHTTP(recorder, r.WithContext(context.WithValue(r.Context(), toolCallContextKey{}, tc)))
	returned = true

	resp := recorder.response()
	if tc.abandoned.Load() {
		s.drop(key, rec)
	} else {
		s.finish(key, rec, resp)
	}
	writeResponse(rw, resp)
}

// toolCallFromRequest parses the tool call headers and chat context of r.
// present is false when r has no tool call headers. badRequest is set
// when the headers are malformed or the chat context is missing.
func toolCallFromRequest(r *http.Request) (key Key, present bool, badRequest *codersdk.Response) {
	tc, present, err := workspacesdk.ToolCallFromHeaders(r.Header)
	if err != nil {
		return Key{}, true, &codersdk.Response{
			Message: "Invalid tool call headers.",
			Detail:  err.Error(),
		}
	}
	if !present {
		return Key{}, false, nil
	}
	chat, ok := agentchat.FromContext(r.Context())
	if !ok {
		return Key{}, true, &codersdk.Response{
			Message: fmt.Sprintf("Tool call headers require the %s header.", workspacesdk.CoderChatIDHeader),
		}
	}
	return Key{
		ChatID:     chat.ID,
		MessageID:  tc.MessageID,
		ToolName:   tc.Name,
		ToolCallID: tc.ID,
	}, true, nil
}

// panicResponse is recorded for a handler that panicked.
var panicResponse = func() response {
	body, _ := json.Marshal(codersdk.Response{Message: "The tool call handler failed."})
	return response{
		status:      http.StatusInternalServerError,
		contentType: "application/json",
		body:        body,
	}
}()

// writeResponse writes a recorded response. Replays carry the status,
// Content-Type, and body of the run and no other headers.
func writeResponse(rw http.ResponseWriter, resp response) {
	if resp.contentType != "" {
		rw.Header().Set("Content-Type", resp.contentType)
	}
	rw.WriteHeader(resp.status)
	_, _ = rw.Write(resp.body)
}

// responseRecorder records the status, Content-Type, and body a handler
// writes.
type responseRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *responseRecorder) Header() http.Header { return w.header }

func (w *responseRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *responseRecorder) Write(b []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.body.Write(b)
}

func (w *responseRecorder) response() response {
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	return response{
		status:      status,
		contentType: w.header.Get("Content-Type"),
		body:        bytes.Clone(w.body.Bytes()),
	}
}

// toolCallErrors maps each decision error to its answer.
var toolCallErrors = []struct {
	err     error
	status  int
	code    workspacesdk.ToolCallErrorCode
	message string
}{
	{errToolCallCanceled, http.StatusConflict, workspacesdk.ToolCallErrorCanceled, "The tool call was canceled before the workspace agent received it."},
	{errToolCallUnknown, http.StatusConflict, workspacesdk.ToolCallErrorUnknown, "The workspace agent cannot tell whether the tool call ran."},
	{errRequestMismatch, http.StatusBadRequest, "", "The request differs from the recorded request for this tool call."},
}

// writeToolCallError writes the answer for a decision error and reports
// whether err was one.
func writeToolCallError(ctx context.Context, rw http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	for _, e := range toolCallErrors {
		if !errors.Is(err, e.err) {
			continue
		}
		if e.code == "" {
			httpapi.Write(ctx, rw, e.status, codersdk.Response{Message: e.message})
			return true
		}
		httpapi.Write(ctx, rw, e.status, workspacesdk.ToolCallError{
			Response: codersdk.Response{Message: e.message},
			Code:     e.code,
		})
		return true
	}
	panic(fmt.Sprintf("developer error: unhandled tool call decision error: %v", err))
}
