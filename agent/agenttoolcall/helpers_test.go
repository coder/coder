package agenttoolcall_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// cutoff is the last_chat_message_id the harness latches.
const cutoff = 100

type harness struct {
	store  *agenttoolcall.Store
	clock  *quartz.Mock
	router http.Handler
	chatID uuid.UUID
}

// newHarness returns a harness whose store has the cutoff message ID
// cutoff and runs next behind the middleware at /start and /other.
func newHarness(t *testing.T, next http.Handler) *harness {
	t.Helper()

	h := newHarnessWithoutCutoff(t, next)
	h.store.SetLastChatMessageID(cutoff)
	return h
}

func newHarnessWithoutCutoff(t *testing.T, next http.Handler) *harness {
	t.Helper()

	clock := quartz.NewMock(t)
	store := agenttoolcall.NewStore(clock)
	r := chi.NewRouter()
	r.Use(agentchat.Middleware)
	r.With(store.Middleware).Handle("/start", next)
	r.With(store.Middleware).Handle("/other", next)
	r.Post("/tool-calls/{id}/cancel", store.CancelHandler())
	return &harness{store: store, clock: clock, router: r, chatID: uuid.New()}
}

func call(messageID int64, name, id string) workspacesdk.ToolCall {
	return workspacesdk.ToolCall{MessageID: messageID, ID: id, Name: name}
}

func (h *harness) key(tc workspacesdk.ToolCall) agenttoolcall.Key {
	return agenttoolcall.Key{ChatID: h.chatID, MessageID: tc.MessageID, ToolName: tc.Name, ToolCallID: tc.ID}
}

func (h *harness) headers(tc workspacesdk.ToolCall) http.Header {
	return toolCallHeaders(h.chatID, tc)
}

func toolCallHeaders(chatID uuid.UUID, tc workspacesdk.ToolCall) http.Header {
	headers := http.Header{workspacesdk.CoderChatIDHeader: {chatID.String()}}
	tc.SetHeaders(headers)
	return headers
}

// do serves a request without test assertions, so it can run in a
// goroutine.
func (h *harness) do(ctx context.Context, method, target string, body io.Reader, headers http.Header) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, method, target, body)
	for k, v := range headers {
		r.Header[k] = v
	}
	h.router.ServeHTTP(w, r)
	return w
}

// start sends POST /start for tc in the harness chat.
func (h *harness) start(ctx context.Context, tc workspacesdk.ToolCall) *httptest.ResponseRecorder {
	return h.do(ctx, http.MethodPost, "/start", strings.NewReader("body"), h.headers(tc))
}

// cancel sends the cancel for tc in the harness chat.
func (h *harness) cancel(ctx context.Context, tc workspacesdk.ToolCall) *httptest.ResponseRecorder {
	return h.cancelIn(ctx, h.chatID, tc)
}

func (h *harness) cancelIn(ctx context.Context, chatID uuid.UUID, tc workspacesdk.ToolCall) *httptest.ResponseRecorder {
	id := workspacesdk.ToolCallUUID(chatID, tc.MessageID, tc.Name, tc.ID)
	return h.do(ctx, http.MethodPost, "/tool-calls/"+id.String()+"/cancel", http.NoBody, toolCallHeaders(chatID, tc))
}

func goDo(f func() *httptest.ResponseRecorder) <-chan *httptest.ResponseRecorder {
	res := make(chan *httptest.ResponseRecorder, 1)
	go func() { res <- f() }()
	return res
}

func (h *harness) advance(t *testing.T, d time.Duration) {
	t.Helper()
	h.clock.Advance(d).MustWait(t.Context())
}

func requireToolCallError(t *testing.T, w *httptest.ResponseRecorder, code workspacesdk.ToolCallErrorCode) {
	t.Helper()

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var resp workspacesdk.ToolCallError
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Equal(t, code, resp.Code)
	require.NotEmpty(t, resp.Message)
}

func requireRecorded(t *testing.T, w *httptest.ResponseRecorder, status int, body string) {
	t.Helper()

	require.Equal(t, status, w.Code, w.Body.String())
	require.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
	require.Equal(t, body, w.Body.String())
}

// countingHandler answers with status (201 when zero), a plain text
// content type, and a body that counts its runs.
type countingHandler struct {
	status int
	calls  atomic.Int32
}

func (h *countingHandler) ServeHTTP(rw http.ResponseWriter, _ *http.Request) {
	n := h.calls.Add(1)
	status := h.status
	if status == 0 {
		status = http.StatusCreated
	}
	writeText(rw, status, fmt.Sprintf("ran %d", n))
}

func writeText(rw http.ResponseWriter, status int, body string) {
	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	rw.WriteHeader(status)
	_, _ = io.WriteString(rw, body)
}

// blockingHandler closes entered on its first run and calls then, or
// answers 201, once release is closed.
type blockingHandler struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	calls   atomic.Int32
	then    func(rw http.ResponseWriter, r *http.Request, n int32)
}

func newBlockingHandler() *blockingHandler {
	return &blockingHandler{entered: make(chan struct{}), release: make(chan struct{})}
}

func (h *blockingHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	n := h.calls.Add(1)
	h.once.Do(func() { close(h.entered) })
	<-h.release
	if h.then != nil {
		h.then(rw, r, n)
		return
	}
	writeText(rw, http.StatusCreated, fmt.Sprintf("done %d", n))
}
