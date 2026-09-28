package agentproc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentproc"
	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

const testToolName = "execute"

func TestToolCallStartRunsOnce(t *testing.T) {
	t.Parallel()

	t.Run("Repeated", func(t *testing.T) {
		t.Parallel()

		env := newToolCallEnv(t, nil)
		chatID := uuid.New()
		runLog := filepath.Join(t.TempDir(), "runs")
		req := workspacesdk.StartProcessRequest{Command: fmt.Sprintf("echo run >> %q", runLog)}
		headers := toolCallHeaders(chatID, 1, "call")

		id := startAndGetID(t, env.handler, req, headers)
		require.Equal(t, toolCallUUID(chatID, 1, "call"), id, "the process ID is the tool call UUID")
		waitForExit(t, env.handler, id)
		require.Equal(t, id, startAndGetID(t, env.handler, req, headers))
		assert.Equal(t, 1, countRuns(t, runLog))
	})

	t.Run("Concurrent", func(t *testing.T) {
		t.Parallel()

		env := newToolCallEnv(t, nil)
		chatID := uuid.New()
		runLog := filepath.Join(t.TempDir(), "runs")
		body, err := json.Marshal(workspacesdk.StartProcessRequest{Command: fmt.Sprintf("echo run >> %q", runLog)})
		require.NoError(t, err)
		ctx := testutil.Context(t, testutil.WaitLong)

		const callers = 8
		responses := make([]*httptest.ResponseRecorder, callers)
		var wg sync.WaitGroup
		for i := range callers {
			wg.Go(func() {
				responses[i] = serve(ctx, env.handler, http.MethodPost, "/start", body, toolCallHeaders(chatID, 1, "call"))
			})
		}
		wg.Wait()

		want := toolCallUUID(chatID, 1, "call")
		for _, w := range responses {
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var resp workspacesdk.StartProcessResponse
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			require.Equal(t, want, resp.ID)
		}
		waitForExit(t, env.handler, want)
		assert.Equal(t, 1, countRuns(t, runLog))
	})

	// A body that cannot be decoded ran nothing, so the next request
	// for the tool call runs it.
	t.Run("AbandonedOnBadBody", func(t *testing.T) {
		t.Parallel()

		env := newToolCallEnv(t, nil)
		chatID := uuid.New()
		runLog := filepath.Join(t.TempDir(), "runs")
		headers := toolCallHeaders(chatID, 1, "call")
		ctx := testutil.Context(t, testutil.WaitLong)

		w := serve(ctx, env.handler, http.MethodPost, "/start", []byte(`{"command":`), headers)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

		id := startAndGetID(t, env.handler, workspacesdk.StartProcessRequest{Command: fmt.Sprintf("echo run >> %q", runLog)}, headers)
		waitForExit(t, env.handler, id)
		assert.Equal(t, 1, countRuns(t, runLog))
	})

	t.Run("WithoutToolCallUnchanged", func(t *testing.T) {
		t.Parallel()

		env := newToolCallEnv(t, nil)
		headers := http.Header{workspacesdk.CoderChatIDHeader: {uuid.New().String()}}
		req := workspacesdk.StartProcessRequest{Command: "true"}

		first := startAndGetID(t, env.handler, req, headers)
		second := startAndGetID(t, env.handler, req, headers)
		assert.NotEqual(t, first, second, "starts without tool call headers are independent")
	})
}

func TestToolCallCancel(t *testing.T) {
	t.Parallel()

	t.Run("KillsRunning", func(t *testing.T) {
		t.Parallel()

		env := newToolCallEnv(t, nil)
		chatID := uuid.New()
		id := startAndGetID(t, env.handler, workspacesdk.StartProcessRequest{Command: "echo before; sleep 300"}, toolCallHeaders(chatID, 1, "call"))
		waitForOutput(t, env.handler, id, "before")
		env.advance(t, 2*time.Second)

		requireCancel(t, env.handler, chatID, 1, "call")
		out := requireOutput(t, env.handler, id)
		assert.False(t, out.Running, "the cancel answers after the process exited")
		assert.True(t, out.Canceled)
		assert.False(t, out.TimedOut)
		assert.EqualValues(t, 2000, out.DurationMs)
		// On Windows, sleep can print a Cygwin startup error when its sh is killed.
		assert.True(t, strings.HasPrefix(out.Output, "before\n"), "output %q must start with the partial output", out.Output)
		require.NotNil(t, out.ExitCode)
		assert.NotZero(t, *out.ExitCode)
	})

	t.Run("Repeated", func(t *testing.T) {
		t.Parallel()

		env := newToolCallEnv(t, nil)
		chatID := uuid.New()
		id := startAndGetID(t, env.handler, workspacesdk.StartProcessRequest{Command: "sleep 300"}, toolCallHeaders(chatID, 1, "call"))

		requireCancel(t, env.handler, chatID, 1, "call")
		first := requireOutput(t, env.handler, id)
		requireCancel(t, env.handler, chatID, 1, "call")
		second := requireOutput(t, env.handler, id)
		assert.True(t, first.Canceled)
		assert.Equal(t, first, second)
	})

	t.Run("ExitedIsNotCanceled", func(t *testing.T) {
		t.Parallel()

		env := newToolCallEnv(t, nil)
		chatID := uuid.New()
		id := startAndGetID(t, env.handler, workspacesdk.StartProcessRequest{Command: "echo done; exit 3"}, toolCallHeaders(chatID, 1, "call"))
		waitForExit(t, env.handler, id)

		requireCancel(t, env.handler, chatID, 1, "call")
		out := requireOutput(t, env.handler, id)
		assert.False(t, out.Canceled)
		assert.Equal(t, "done\n", out.Output)
		require.NotNil(t, out.ExitCode)
		assert.Equal(t, 3, *out.ExitCode)
	})

	// A kill through process_signal is the model's own action, not a
	// cancel.
	t.Run("SignalKillIsNotCanceled", func(t *testing.T) {
		t.Parallel()

		env := newToolCallEnv(t, nil)
		chatID := uuid.New()
		id := startAndGetID(t, env.handler, workspacesdk.StartProcessRequest{Command: "sleep 300"}, toolCallHeaders(chatID, 1, "call"))
		w := postSignal(t, env.handler, id, workspacesdk.SignalProcessRequest{Signal: "kill"})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		waitForExit(t, env.handler, id)

		requireCancel(t, env.handler, chatID, 1, "call")
		assert.False(t, requireOutput(t, env.handler, id).Canceled)
	})

	// The cancel lands while the start is between creating its record
	// and spawning the process, so the start registers its cancel hook
	// on a canceled record.
	t.Run("RacesStart", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			env := newToolCallEnv(t, func(env []string) ([]string, error) {
				once.Do(func() {
					close(entered)
					<-release
				})
				return env, nil
			})
			chatID := uuid.New()
			body, err := json.Marshal(workspacesdk.StartProcessRequest{Command: "sleep 300"})
			require.NoError(t, err)

			started := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				started <- serve(t.Context(), env.handler, http.MethodPost, "/start", body, toolCallHeaders(chatID, 1, "call"))
			}()
			<-entered
			canceled := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				canceled <- postCancel(t.Context(), env.handler, chatID, 1, "call")
			}()
			synctest.Wait()
			require.Empty(t, canceled, "the cancel waits for the pending start")

			close(release)
			w := <-canceled
			require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
			w = <-started
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			out := requireOutput(t, env.handler, toolCallUUID(chatID, 1, "call"))
			assert.False(t, out.Running)
			assert.True(t, out.Canceled, "the process the start spawned is killed")
		})
	})
}

func TestToolCallExecuteDeadline(t *testing.T) {
	t.Parallel()

	const maxWait = 5 * time.Minute
	tests := []struct {
		name         string
		timeout      time.Duration
		wantWait     time.Duration
		wantTimedOut bool
	}{
		{name: "EndsAtDeadline", timeout: 10 * time.Second, wantWait: 10 * time.Second, wantTimedOut: true},
		{name: "NoDeadlineEndsAtCap", wantWait: maxWait},
		{name: "CapBeforeDeadline", timeout: 2 * maxWait, wantWait: maxWait},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := newToolCallEnv(t, nil)
			ctx := testutil.Context(t, testutil.WaitLong)
			chatID := uuid.New()
			req := workspacesdk.StartProcessRequest{Command: "sleep 300", TimeoutMs: tt.timeout.Milliseconds()}
			headers := toolCallHeaders(chatID, 1, "call")
			id := startAndGetID(t, env.handler, req, headers)
			t.Cleanup(func() {
				postSignal(t, env.handler, id, workspacesdk.SignalProcessRequest{Signal: "kill"})
			})

			out := env.waitOutput(ctx, t, id, tt.wantWait)
			assert.True(t, out.Running)
			assert.Equal(t, tt.wantTimedOut, out.TimedOut)
			assert.Equal(t, tt.wantWait.Milliseconds(), out.DurationMs)
		})
	}

	// A replayed start returns the recorded response, so the deadline
	// stays where the first start put it.
	t.Run("ReplayDoesNotExtend", func(t *testing.T) {
		t.Parallel()

		env := newToolCallEnv(t, nil)
		ctx := testutil.Context(t, testutil.WaitLong)
		chatID := uuid.New()
		req := workspacesdk.StartProcessRequest{Command: "sleep 300", TimeoutMs: (10 * time.Second).Milliseconds()}
		headers := toolCallHeaders(chatID, 1, "call")
		id := startAndGetID(t, env.handler, req, headers)
		t.Cleanup(func() {
			postSignal(t, env.handler, id, workspacesdk.SignalProcessRequest{Signal: "kill"})
		})

		env.advance(t, 6*time.Second)
		require.Equal(t, id, startAndGetID(t, env.handler, req, headers))
		out := env.waitOutput(ctx, t, id, 4*time.Second)
		assert.True(t, out.TimedOut)
		assert.EqualValues(t, 10000, out.DurationMs)

		// Past the deadline the wait answers at once.
		w := serve(ctx, env.handler, http.MethodGet, fmt.Sprintf("/%s/output?wait=true", id), nil, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var again workspacesdk.ProcessOutputResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&again))
		assert.True(t, again.Running)
		assert.True(t, again.TimedOut)
	})

	t.Run("ExitBeforeDeadline", func(t *testing.T) {
		t.Parallel()

		env := newToolCallEnv(t, nil)
		chatID := uuid.New()
		req := workspacesdk.StartProcessRequest{Command: "exit 0", TimeoutMs: (10 * time.Second).Milliseconds()}
		id := startAndGetID(t, env.handler, req, toolCallHeaders(chatID, 1, "call"))
		waitForExit(t, env.handler, id)
		env.advance(t, 20*time.Second)
		out := requireOutput(t, env.handler, id)
		assert.False(t, out.Running)
		assert.False(t, out.TimedOut)
		assert.Zero(t, out.DurationMs)
	})
}

func TestToolCallProcessReaping(t *testing.T) {
	t.Parallel()

	env := newToolCallEnv(t, nil)
	chatID := uuid.New()
	chatHeaders := http.Header{workspacesdk.CoderChatIDHeader: {chatID.String()}}

	keyed := startAndGetID(t, env.handler, workspacesdk.StartProcessRequest{Command: "true"}, toolCallHeaders(chatID, 1, "call"))
	unkeyed := startAndGetID(t, env.handler, workspacesdk.StartProcessRequest{Command: "true"}, chatHeaders)
	waitForExit(t, env.handler, keyed)
	waitForExit(t, env.handler, unkeyed)

	env.advance(t, 6*time.Minute)
	assert.Equal(t, http.StatusNotFound, getOutput(t, env.handler, unkeyed).Code, "the periodic sweep reaps an exited process without a tool call after 5 minutes")
	assert.Equal(t, http.StatusOK, getOutput(t, env.handler, keyed).Code, "the store retains the exited tool call process")

	// The periodic sweep evicts the idle record and reaps its process
	// without a list request.
	env.advance(t, time.Hour)
	assert.Equal(t, http.StatusNotFound, getOutput(t, env.handler, keyed).Code)
}

func TestToolCallCloseKills(t *testing.T) {
	t.Parallel()

	env := newToolCallEnv(t, nil)
	id := startAndGetID(t, env.handler, workspacesdk.StartProcessRequest{Command: "sleep 300"}, toolCallHeaders(uuid.New(), 1, "call"))

	require.NoError(t, env.api.Close())
	out := requireOutput(t, env.handler, id)
	assert.False(t, out.Running)
	assert.NotNil(t, out.ExitCode)
	assert.False(t, out.Canceled, "agent shutdown is not a cancel")
}

func TestToolCallChatIsolation(t *testing.T) {
	t.Parallel()

	env := newToolCallEnv(t, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	chatA := uuid.New()
	chatB := uuid.New()
	idA := startAndGetID(t, env.handler, workspacesdk.StartProcessRequest{Command: "sleep 300"}, toolCallHeaders(chatA, 1, "call"))
	t.Cleanup(func() {
		postSignal(t, env.handler, idA, workspacesdk.SignalProcessRequest{Signal: "kill"})
	})
	headersB := http.Header{workspacesdk.CoderChatIDHeader: {chatB.String()}}

	w := getOutputWithHeaders(t, env.handler, idA, headersB)
	assert.Equal(t, http.StatusNotFound, w.Code, "chat B cannot read chat A's process")
	body, err := json.Marshal(workspacesdk.SignalProcessRequest{Signal: "kill"})
	require.NoError(t, err)
	w = serve(ctx, env.handler, http.MethodPost, fmt.Sprintf("/%s/signal", idA), body, headersB)
	assert.Equal(t, http.StatusNotFound, w.Code, "chat B cannot signal chat A's process")
	w = serve(ctx, env.handler, http.MethodPost, fmt.Sprintf("/tool-calls/%s/cancel", idA), nil, toolCallHeaders(chatB, 1, "call"))
	assert.Equal(t, http.StatusBadRequest, w.Code, "chat B cannot cancel chat A's tool call")

	// The same tool call in chat B is a different tool call.
	runLog := filepath.Join(t.TempDir(), "runs")
	idB := startAndGetID(t, env.handler, workspacesdk.StartProcessRequest{Command: fmt.Sprintf("echo run >> %q", runLog)}, toolCallHeaders(chatB, 1, "call"))
	assert.NotEqual(t, idA, idB)
	waitForExit(t, env.handler, idB)
	assert.Equal(t, 1, countRuns(t, runLog))
	requireCancel(t, env.handler, chatB, 1, "call")

	assert.True(t, requireOutput(t, env.handler, idA).Running, "chat A's process keeps running")
}

type toolCallEnv struct {
	api     *agentproc.API
	handler http.Handler
	clock   *quartz.Mock
}

// newToolCallEnv returns a process API with a tool call store on a mock
// clock, served the way the agent mounts it. Every message ID is above
// the store's cutoff. updateEnv may be nil.
func newToolCallEnv(t *testing.T, updateEnv func([]string) ([]string, error)) *toolCallEnv {
	t.Helper()

	clock := quartz.NewMock(t)
	store := agenttoolcall.NewStore(clock)
	store.SetLastChatMessageID(0)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	api := agentproc.NewAPI(logger, agentexec.DefaultExecer, nil, nil, nil, updateEnv, nil, agentproc.WithClock(clock), agentproc.WithToolCallStore(store))
	t.Cleanup(func() {
		_ = api.Close()
	})

	router := chi.NewRouter()
	router.Post("/tool-calls/{id}/cancel", store.CancelHandler())
	router.Mount("/", api.Routes())
	return &toolCallEnv{api: api, handler: agentchat.Middleware(router), clock: clock}
}

// advance moves the mock clock forward by d in steps that stop at every
// timer and tick, as quartz requires.
func (e *toolCallEnv) advance(t *testing.T, d time.Duration) {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitShort)
	for d > 0 {
		step := d
		if next, ok := e.clock.Peek(); ok && next > 0 && next < step {
			step = next
		}
		e.clock.Advance(step).MustWait(ctx)
		d -= step
	}
}

// waitOutput sends an output request with wait=true, advances the clock
// by wantWait once the request waits, and returns the answer.
func (e *toolCallEnv) waitOutput(ctx context.Context, t *testing.T, id string, wantWait time.Duration) workspacesdk.ProcessOutputResponse {
	t.Helper()

	trap := e.clock.Trap().AfterFunc("agentproc", "wait")
	defer trap.Close()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- serve(ctx, e.handler, http.MethodGet, fmt.Sprintf("/%s/output?wait=true", id), nil, nil)
	}()
	call := trap.MustWait(ctx)
	require.Equal(t, wantWait, call.Duration)
	call.MustRelease(ctx)
	e.advance(t, wantWait)

	w := testutil.RequireReceive(ctx, t, done)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp workspacesdk.ProcessOutputResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

// serve serves a request without test assertions, so it can run in a
// goroutine.
func serve(ctx context.Context, handler http.Handler, method, path string, body []byte, headers http.Header) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
	for k, v := range headers {
		r.Header[k] = v
	}
	handler.ServeHTTP(w, r)
	return w
}

func toolCallHeaders(chatID uuid.UUID, messageID int64, toolCallID string) http.Header {
	h := http.Header{workspacesdk.CoderChatIDHeader: {chatID.String()}}
	workspacesdk.ToolCall{MessageID: messageID, ID: toolCallID, Name: testToolName}.SetHeaders(h)
	return h
}

func toolCallUUID(chatID uuid.UUID, messageID int64, toolCallID string) string {
	return workspacesdk.ToolCallUUID(chatID, messageID, testToolName, toolCallID).String()
}

func postCancel(ctx context.Context, handler http.Handler, chatID uuid.UUID, messageID int64, toolCallID string) *httptest.ResponseRecorder {
	path := fmt.Sprintf("/tool-calls/%s/cancel", toolCallUUID(chatID, messageID, toolCallID))
	return serve(ctx, handler, http.MethodPost, path, nil, toolCallHeaders(chatID, messageID, toolCallID))
}

func requireCancel(t *testing.T, handler http.Handler, chatID uuid.UUID, messageID int64, toolCallID string) {
	t.Helper()

	w := postCancel(testutil.Context(t, testutil.WaitLong), handler, chatID, messageID, toolCallID)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
}

func requireOutput(t *testing.T, handler http.Handler, id string) workspacesdk.ProcessOutputResponse {
	t.Helper()

	w := getOutput(t, handler, id)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp workspacesdk.ProcessOutputResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

// countRuns returns the number of lines in the run log, zero if the
// command never ran.
func countRuns(t *testing.T, runLog string) int {
	t.Helper()

	data, err := os.ReadFile(runLog)
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	return strings.Count(string(data), "\n")
}

// waitForOutput polls the output endpoint until the output contains want.
func waitForOutput(t *testing.T, handler http.Handler, id, want string) {
	t.Helper()

	require.Eventually(t, func() bool {
		w := getOutput(t, handler, id)
		var resp workspacesdk.ProcessOutputResponse
		return json.NewDecoder(w.Body).Decode(&resp) == nil && strings.Contains(resp.Output, want)
	}, testutil.WaitLong, testutil.IntervalFast)
}
