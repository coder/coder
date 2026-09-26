package agenttoolcall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// ToolCall is the tool call a request runs for, as seen by the handler
// behind Middleware.
type ToolCall struct {
	Key Key
	// UUID is workspacesdk.ToolCallUUID of Key, the ID of anything the
	// tool call creates, such as a process.
	UUID uuid.UUID
}

type toolCallContextKey struct{}

// FromContext returns the tool call that Middleware put in the context of
// a handler it runs.
func FromContext(ctx context.Context) (ToolCall, bool) {
	tc, ok := ctx.Value(toolCallContextKey{}).(ToolCall)
	return tc, ok
}

// Middleware runs next at most once per tool call and records its
// response. A request without tool call headers goes to next unchanged. A
// request with them gets the recorded response of the tool call's one run,
// waiting for a run in progress, or a 409 when the agent must not run it.
// The request's method, URL path, raw query, and body must match the
// recorded request.
func (s *Store) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		key, age, present, badRequest := toolCallFromRequest(r)
		if !present {
			next.ServeHTTP(rw, r)
			return
		}
		if badRequest != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, *badRequest)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to read request body.",
				Detail:  err.Error(),
			})
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		rec, created, err := s.begin(key, age, requestInput(r, body))
		if writeToolCallError(ctx, rw, err) {
			return
		}
		if created {
			tc := ToolCall{Key: key, UUID: workspacesdk.ToolCallUUID(key.ChatID, key.MessageID, key.ToolCallID)}
			rec.run(next, r.WithContext(context.WithValue(ctx, toolCallContextKey{}, tc)))
		}
		resp, ok := rec.wait(ctx)
		if !ok {
			// The client is gone.
			return
		}
		s.writeRecorded(rw, rec, resp)
	})
}

// toolCallFromRequest parses the tool call headers and chat context of r.
// present is false when r has no tool call headers. badRequest is set when
// the headers are malformed or the chat context is missing.
func toolCallFromRequest(r *http.Request) (key Key, age time.Duration, present bool, badRequest *codersdk.Response) {
	tc, present, err := workspacesdk.ToolCallFromHeaders(r.Header)
	if err != nil {
		return Key{}, 0, true, &codersdk.Response{
			Message: "Invalid tool call headers.",
			Detail:  err.Error(),
		}
	}
	if !present {
		return Key{}, 0, false, nil
	}
	chat, ok := agentchat.FromContext(r.Context())
	if !ok {
		return Key{}, 0, true, &codersdk.Response{
			Message: fmt.Sprintf("Tool call headers require the %s header.", workspacesdk.CoderChatIDHeader),
		}
	}
	return Key{ChatID: chat.ID, MessageID: tc.MessageID, ToolCallID: tc.ID}, tc.Age, true, nil
}

// requestInput hashes what a repeated request must match. Each field is
// prefixed with its length so that different requests cannot encode to
// the same bytes.
func requestInput(r *http.Request, body []byte) [sha256.Size]byte {
	h := sha256.New()
	for _, field := range [][]byte{[]byte(r.Method), []byte(r.URL.Path), []byte(r.URL.RawQuery), body} {
		_ = binary.Write(h, binary.BigEndian, uint64(len(field)))
		_, _ = h.Write(field)
	}
	var sum [sha256.Size]byte
	h.Sum(sum[:0])
	return sum
}

// run runs next and publishes its response. If next panics, rec gets a 500
// response so waiters and later requests do not wait forever, and the
// panic continues.
func (rec *record) run(next http.Handler, r *http.Request) {
	recorder := &responseRecorder{header: http.Header{}}
	returned := false
	defer func() {
		if !returned {
			rec.publish(panicResponse)
		}
	}()
	next.ServeHTTP(recorder, r)
	returned = true
	rec.publish(recorder.response())
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

// writeRecorded writes resp, the recorded response of rec.
func (s *Store) writeRecorded(rw http.ResponseWriter, rec *record, resp response) {
	if resp.contentType != "" {
		rw.Header().Set("Content-Type", resp.contentType)
	}
	rw.Header().Set(workspacesdk.CoderToolCallRunAgeMsHeader, strconv.FormatInt(s.runAge(rec).Milliseconds(), 10))
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
		body:        w.body.Bytes(),
	}
}

// toolCallErrors maps each decision error to its code and message.
var toolCallErrors = []struct {
	err     error
	code    workspacesdk.ToolCallErrorCode
	message string
}{
	{errStaleToolCall, workspacesdk.ToolCallErrorStale, "The tool call is in an older message than the chat's latest message."},
	{errAgentStartedAfterToolCall, workspacesdk.ToolCallErrorAgentStartedAfterToolCall, "The workspace agent started after the tool call was committed."},
	{errInputMismatch, workspacesdk.ToolCallErrorInputMismatch, "The request differs from the recorded request for this tool call."},
	{errToolCallCanceled, workspacesdk.ToolCallErrorCanceled, "The tool call was canceled."},
}

// writeToolCallError writes the HTTP 409 for a decision error and reports
// whether err was one.
func writeToolCallError(ctx context.Context, rw http.ResponseWriter, err error) bool {
	for _, e := range toolCallErrors {
		if errors.Is(err, e.err) {
			httpapi.Write(ctx, rw, http.StatusConflict, workspacesdk.ToolCallError{
				Response: codersdk.Response{Message: e.message},
				Code:     e.code,
			})
			return true
		}
	}
	return false
}
