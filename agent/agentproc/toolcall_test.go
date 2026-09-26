package agentproc_test

import (
	"bytes"
	"cmp"
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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentgit"
	"github.com/coder/coder/v2/agent/agentproc"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// longRunning is how long the agent has been running in tests that need
// every tool call to be provably new to the agent.
const longRunning = time.Hour

func TestStartProcessToolCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// uptime is how long the agent has been running. Zero means
		// longRunning.
		uptime time.Duration
		// setup sends earlier requests for the chat. appendCmd appends a
		// line to the run log.
		setup     func(t *testing.T, handler http.Handler, chatID uuid.UUID, appendCmd string)
		messageID int64
		age       time.Duration
		wantCode  int
		wantError workspacesdk.ToolCallErrorCode
		// wantRuns is how many times the command ran after the request.
		wantRuns int
	}{
		{
			name:      "New",
			messageID: 1,
			wantCode:  http.StatusOK,
			wantRuns:  1,
		},
		{
			name: "RepeatedSameInput",
			setup: func(t *testing.T, handler http.Handler, chatID uuid.UUID, appendCmd string) {
				id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: appendCmd}, toolCallHeaders(chatID, 1, "call", 0))
				waitForExit(t, handler, id)
			},
			messageID: 1,
			wantCode:  http.StatusOK,
			wantRuns:  1,
		},
		{
			name: "InputMismatch",
			setup: func(t *testing.T, handler http.Handler, chatID uuid.UUID, appendCmd string) {
				id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: appendCmd, Background: true}, toolCallHeaders(chatID, 1, "call", 0))
				waitForExit(t, handler, id)
			},
			messageID: 1,
			wantCode:  http.StatusConflict,
			wantError: workspacesdk.ToolCallErrorInputMismatch,
			wantRuns:  1,
		},
		{
			name: "StaleMessage",
			setup: func(t *testing.T, handler http.Handler, chatID uuid.UUID, _ string) {
				startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "true"}, toolCallHeaders(chatID, 2, "other", 0))
			},
			messageID: 1,
			wantCode:  http.StatusConflict,
			wantError: workspacesdk.ToolCallErrorStale,
		},
		{
			name:      "AgentStartedAfterToolCall",
			uptime:    10 * time.Second,
			messageID: 1,
			age:       time.Minute,
			wantCode:  http.StatusConflict,
			wantError: workspacesdk.ToolCallErrorAgentStartedAfterToolCall,
		},
		{
			name: "CanceledBeforeStart",
			setup: func(t *testing.T, handler http.Handler, chatID uuid.UUID, _ string) {
				resp := requireCancel(t, handler, chatID, 1, "call", 0)
				require.False(t, resp.Started)
			},
			messageID: 1,
			wantCode:  http.StatusConflict,
			wantError: workspacesdk.ToolCallErrorCanceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newToolCallTestAPI(t, cmp.Or(tt.uptime, longRunning))
			chatID := uuid.New()
			runLog := filepath.Join(t.TempDir(), "runs")
			appendCmd := fmt.Sprintf("echo run >> %q", runLog)
			if tt.setup != nil {
				tt.setup(t, handler, chatID, appendCmd)
			}

			w := postStart(t, handler, workspacesdk.StartProcessRequest{Command: appendCmd}, toolCallHeaders(chatID, tt.messageID, "call", tt.age))
			require.Equal(t, tt.wantCode, w.Code, w.Body.String())
			if tt.wantCode == http.StatusOK {
				var resp workspacesdk.StartProcessResponse
				require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
				require.True(t, resp.Started)
				require.Equal(t, workspacesdk.ToolCallUUID(chatID, tt.messageID, "call").String(), resp.ID)
				waitForExit(t, handler, resp.ID)
			} else {
				require.Equal(t, tt.wantError, decodeToolCallError(t, w).Code)
			}
			assert.Equal(t, tt.wantRuns, countRuns(t, runLog))
		})
	}
}

func TestStartProcessToolCallConcurrent(t *testing.T) {
	t.Parallel()

	handler, _ := newToolCallTestAPI(t, longRunning)
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
			responses[i] = serveToolCallRequest(ctx, handler, http.MethodPost, "/start", body, toolCallHeaders(chatID, 1, "call", 0))
		})
	}
	wg.Wait()

	want := workspacesdk.ToolCallUUID(chatID, 1, "call").String()
	for _, w := range responses {
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp workspacesdk.StartProcessResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		require.Equal(t, want, resp.ID)
	}
	waitForExit(t, handler, want)
	require.Equal(t, 1, countRuns(t, runLog))
}

func TestStartProcessToolCallStartError(t *testing.T) {
	t.Parallel()

	handler, _ := newToolCallTestAPI(t, longRunning)
	chatID := uuid.New()
	req := workspacesdk.StartProcessRequest{
		Command: "true",
		WorkDir: filepath.Join(t.TempDir(), "missing"),
	}

	for range 2 {
		w := postStart(t, handler, req, toolCallHeaders(chatID, 1, "call", 0))
		require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	}

	// An aborted request must not be answered as if no process started.
	aborted, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
	cancel()
	id := workspacesdk.ToolCallUUID(chatID, 1, "call").String()
	w := postCancelContext(aborted, handler, id, toolCallHeaders(chatID, 1, "call", 0))
	assert.Empty(t, w.Body.String())

	resp := requireCancel(t, handler, chatID, 1, "call", 0)
	assert.False(t, resp.Started, "a recorded start error means no process started")
}

func TestStartProcessWithoutToolCallUnchanged(t *testing.T) {
	t.Parallel()

	handler, _ := newToolCallTestAPI(t, longRunning)
	headers := http.Header{workspacesdk.CoderChatIDHeader: {uuid.New().String()}}
	req := workspacesdk.StartProcessRequest{Command: "true"}

	first := startAndGetID(t, handler, req, headers)
	second := startAndGetID(t, handler, req, headers)
	require.NotEqual(t, first, second, "starts without tool call headers are independent")
}

func TestToolCallBadRequest(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	id := workspacesdk.ToolCallUUID(chatID, 1, "call").String()
	withChat := func(h http.Header) http.Header {
		h.Set(workspacesdk.CoderChatIDHeader, chatID.String())
		return h
	}
	toolCallOnly := func() http.Header {
		h := http.Header{}
		workspacesdk.ToolCall{MessageID: 1, ID: "call"}.SetHeaders(h)
		return h
	}
	partial := http.Header{workspacesdk.CoderToolCallMessageIDHeader: {"1"}}
	malformedAge := toolCallOnly()
	malformedAge.Set(workspacesdk.CoderToolCallAgeMsHeader, "-1")

	tests := []struct {
		name    string
		cancel  bool
		id      string
		headers http.Header
	}{
		{name: "StartWithoutChat", headers: toolCallOnly()},
		{name: "StartPartialHeaders", headers: withChat(partial.Clone())},
		{name: "StartMalformedAge", headers: withChat(malformedAge.Clone())},
		{name: "CancelWithoutChat", cancel: true, id: id, headers: toolCallOnly()},
		{name: "CancelWithoutToolCall", cancel: true, id: id, headers: withChat(http.Header{})},
		{name: "CancelPartialHeaders", cancel: true, id: id, headers: withChat(partial.Clone())},
		{name: "CancelMalformedAge", cancel: true, id: id, headers: withChat(malformedAge.Clone())},
		{name: "CancelWrongID", cancel: true, id: uuid.New().String(), headers: toolCallHeaders(chatID, 1, "call", 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newToolCallTestAPI(t, longRunning)
			var w *httptest.ResponseRecorder
			if tt.cancel {
				w = postCancel(t, handler, tt.id, tt.headers)
			} else {
				w = postStart(t, handler, workspacesdk.StartProcessRequest{Command: "true"}, tt.headers)
			}
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			// Nothing was recorded, so a valid request still starts.
			startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "true"}, toolCallHeaders(chatID, 1, "call", 0))
		})
	}
}

func TestCancelProcess(t *testing.T) {
	t.Parallel()

	t.Run("Running", func(t *testing.T) {
		t.Parallel()

		handler, clock := newToolCallTestAPI(t, longRunning)
		chatID := uuid.New()
		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "echo before; sleep 300"}, toolCallHeaders(chatID, 1, "call", 0))
		waitForOutput(t, handler, id, "before")
		clock.Advance(2 * time.Second).MustWait(testutil.Context(t, testutil.WaitShort))

		resp := requireCancel(t, handler, chatID, 1, "call", 0)
		assert.True(t, resp.Started)
		assert.True(t, resp.Canceled)
		assert.Equal(t, "before\n", resp.Output)
		require.NotNil(t, resp.ExitCode)
		assert.NotZero(t, *resp.ExitCode)
		assert.EqualValues(t, 2000, resp.AgeMs)

		out := waitForExit(t, handler, id)
		assert.Equal(t, resp.ExitCode, out.ExitCode)
	})

	// An interrupt task retry sends the cancel again after the process
	// exited from the first one.
	t.Run("RepeatedCancel", func(t *testing.T) {
		t.Parallel()

		handler, _ := newToolCallTestAPI(t, longRunning)
		chatID := uuid.New()
		startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "echo before; sleep 300"}, toolCallHeaders(chatID, 1, "call", 0))

		first := requireCancel(t, handler, chatID, 1, "call", 0)
		second := requireCancel(t, handler, chatID, 1, "call", 0)
		assert.True(t, first.Canceled)
		assert.True(t, second.Canceled, "a repeated cancel must still report that the user canceled the process")
		assert.Equal(t, first, second)
	})

	t.Run("Exited", func(t *testing.T) {
		t.Parallel()

		handler, _ := newToolCallTestAPI(t, longRunning)
		chatID := uuid.New()
		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "echo done; exit 3"}, toolCallHeaders(chatID, 1, "call", 0))
		waitForExit(t, handler, id)

		resp := requireCancel(t, handler, chatID, 1, "call", 0)
		assert.True(t, resp.Started)
		assert.False(t, resp.Canceled)
		assert.Equal(t, "done\n", resp.Output)
		require.NotNil(t, resp.ExitCode)
		assert.Equal(t, 3, *resp.ExitCode)
	})

	t.Run("AbortedRequestStillKills", func(t *testing.T) {
		t.Parallel()

		handler, _ := newToolCallTestAPI(t, longRunning)
		chatID := uuid.New()
		headers := toolCallHeaders(chatID, 1, "call", 0)
		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "sleep 300"}, headers)

		aborted, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		cancel()
		postCancelContext(aborted, handler, id, headers)
		out := waitForExit(t, handler, id)
		assert.NotNil(t, out.ExitCode)
		// The retried cancel reports the kill the aborted one sent.
		resp := requireCancel(t, handler, chatID, 1, "call", 0)
		assert.True(t, resp.Canceled)
	})

	t.Run("NeverReceived", func(t *testing.T) {
		t.Parallel()

		handler, _ := newToolCallTestAPI(t, longRunning)
		chatID := uuid.New()
		for range 2 {
			resp := requireCancel(t, handler, chatID, 1, "call", 0)
			assert.Equal(t, workspacesdk.CancelProcessResponse{}, resp)
		}
	})

	t.Run("AgentStartedAfterToolCall", func(t *testing.T) {
		t.Parallel()

		handler, _ := newToolCallTestAPI(t, 10*time.Second)
		chatID := uuid.New()
		w := postCancel(t, handler, workspacesdk.ToolCallUUID(chatID, 1, "call").String(), toolCallHeaders(chatID, 1, "call", time.Minute))
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		require.Equal(t, workspacesdk.ToolCallErrorAgentStartedAfterToolCall, decodeToolCallError(t, w).Code)
	})

	t.Run("StaleMessage", func(t *testing.T) {
		t.Parallel()

		handler, _ := newToolCallTestAPI(t, longRunning)
		chatID := uuid.New()
		startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "true"}, toolCallHeaders(chatID, 2, "other", 0))
		w := postCancel(t, handler, workspacesdk.ToolCallUUID(chatID, 1, "call").String(), toolCallHeaders(chatID, 1, "call", 0))
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		require.Equal(t, workspacesdk.ToolCallErrorStale, decodeToolCallError(t, w).Code)
	})

	t.Run("OtherChat", func(t *testing.T) {
		t.Parallel()

		handler, _ := newToolCallTestAPI(t, longRunning)
		chatA := uuid.New()
		chatB := uuid.New()
		idA := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "sleep 300"}, toolCallHeaders(chatA, 1, "call", 0))
		t.Cleanup(func() {
			postSignal(t, handler, idA, workspacesdk.SignalProcessRequest{Signal: "kill"})
		})

		// Chat B cannot address chat A's process ID.
		w := postCancel(t, handler, idA, toolCallHeaders(chatB, 1, "call", 0))
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		// The same tool call identifiers in chat B are a different tool call.
		resp := requireCancel(t, handler, chatB, 1, "call", 0)
		assert.False(t, resp.Started)

		out := getOutput(t, handler, idA)
		require.Equal(t, http.StatusOK, out.Code)
		var outResp workspacesdk.ProcessOutputResponse
		require.NoError(t, json.NewDecoder(out.Body).Decode(&outResp))
		assert.True(t, outResp.Running, "chat A's process must keep running")
	})
}

func TestToolCallProcessAge(t *testing.T) {
	t.Parallel()

	handler, clock := newToolCallTestAPI(t, longRunning)
	chatID := uuid.New()
	ctx := testutil.Context(t, testutil.WaitShort)

	w := postStart(t, handler, workspacesdk.StartProcessRequest{Command: "true"}, toolCallHeaders(chatID, 1, "call", 0))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var start workspacesdk.StartProcessResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&start))
	assert.Zero(t, start.AgeMs)

	clock.Advance(1500 * time.Millisecond).MustWait(ctx)
	out := waitForExit(t, handler, start.ID)
	assert.EqualValues(t, 1500, out.AgeMs)

	clock.Advance(time.Second).MustWait(ctx)
	w = postStart(t, handler, workspacesdk.StartProcessRequest{Command: "true"}, toolCallHeaders(chatID, 1, "call", 0))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var again workspacesdk.StartProcessResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&again))
	assert.Equal(t, start.ID, again.ID)
	assert.EqualValues(t, 2500, again.AgeMs)
}

func TestToolCallProcessReaping(t *testing.T) {
	t.Parallel()

	handler, clock := newToolCallTestAPI(t, longRunning)
	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	chatHeaders := http.Header{workspacesdk.CoderChatIDHeader: {chatID.String()}}

	keyed := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "true"}, toolCallHeaders(chatID, 1, "call", 0))
	unkeyed := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "true"}, chatHeaders)
	waitForExit(t, handler, keyed)
	waitForExit(t, handler, unkeyed)

	clock.Advance(6 * time.Minute).MustWait(ctx)
	ids := listIDs(t, handler)
	assert.Contains(t, ids, keyed, "an exited process with a current tool call record is kept")
	assert.NotContains(t, ids, unkeyed)

	newer := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "true"}, toolCallHeaders(chatID, 2, "call", 0))
	ids = listIDs(t, handler)
	assert.NotContains(t, ids, keyed, "a newer message ends the older record, so the 5-minute rule applies")
	assert.Contains(t, ids, newer)
}

func TestCancelProcessWaitsForPendingStart(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// Blocking updateEnv holds the start between inserting its record
		// and spawning the process.
		entered := make(chan struct{})
		release := make(chan struct{})
		var once sync.Once
		updateEnv := func(env []string) ([]string, error) {
			once.Do(func() {
				close(entered)
				<-release
			})
			return env, nil
		}
		api, _ := newToolCallAPI(t, longRunning, nil, updateEnv)
		handler := agentchat.Middleware(api.Routes())
		chatID := uuid.New()
		headers := toolCallHeaders(chatID, 1, "call", 0)
		body, err := json.Marshal(workspacesdk.StartProcessRequest{Command: "sleep 300"})
		require.NoError(t, err)

		started := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			started <- serveToolCallRequest(t.Context(), handler, http.MethodPost, "/start", body, headers)
		}()
		<-entered
		canceled := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			canceled <- postCancelContext(t.Context(), handler, workspacesdk.ToolCallUUID(chatID, 1, "call").String(), headers)
		}()
		synctest.Wait()
		require.Empty(t, canceled, "cancel must wait for the pending start")

		close(release)
		w := <-canceled
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp workspacesdk.CancelProcessResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Started)
		assert.True(t, resp.Canceled, "the started process must be killed")
		w = <-started
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})
}

func TestCloseKillsToolCallProcess(t *testing.T) {
	t.Parallel()

	api, _ := newToolCallAPI(t, longRunning, nil, nil)
	handler := agentchat.Middleware(api.Routes())
	id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "sleep 300"}, toolCallHeaders(uuid.New(), 1, "call", 0))

	require.NoError(t, api.Close())
	w := getOutput(t, handler, id)
	require.Equal(t, http.StatusOK, w.Code)
	var resp workspacesdk.ProcessOutputResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.False(t, resp.Running)
	assert.NotNil(t, resp.ExitCode)
}

func TestRepeatedStartNotifiesGitOnce(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		pathStore := agentgit.NewPathStore()
		chatID := uuid.New()
		notified, unsubscribe := pathStore.Subscribe(chatID)
		defer unsubscribe()
		api, _ := newToolCallAPI(t, longRunning, pathStore, nil)
		handler := agentchat.Middleware(api.Routes())
		req := workspacesdk.StartProcessRequest{Command: "true"}
		headers := toolCallHeaders(chatID, 1, "call", 0)

		// synctest.Wait returns once the process has exited and every
		// watcher goroutine has finished.
		id := startAndGetID(t, handler, req, headers)
		synctest.Wait()
		select {
		case <-notified:
		default:
			t.Fatal("no git notification after the process exited")
		}

		require.Equal(t, id, startAndGetID(t, handler, req, headers))
		synctest.Wait()
		select {
		case <-notified:
			t.Fatal("a repeated start notified git again")
		default:
		}
	})
}

// newToolCallTestAPI returns a handler whose agent has been running for
// uptime on the returned mock clock.
func newToolCallTestAPI(t *testing.T, uptime time.Duration) (http.Handler, *quartz.Mock) {
	t.Helper()

	api, clock := newToolCallAPI(t, uptime, nil, nil)
	return agentchat.Middleware(api.Routes()), clock
}

// newToolCallAPI returns an API whose agent has been running for uptime
// on the returned mock clock. pathStore and updateEnv may be nil.
func newToolCallAPI(t *testing.T, uptime time.Duration, pathStore *agentgit.PathStore, updateEnv func([]string) ([]string, error)) (*agentproc.API, *quartz.Mock) {
	t.Helper()

	clock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	api := agentproc.NewAPI(logger, agentexec.DefaultExecer, nil, pathStore, nil, updateEnv, nil, agentproc.WithClock(clock))
	t.Cleanup(func() {
		_ = api.Close()
	})
	clock.Advance(uptime).MustWait(testutil.Context(t, testutil.WaitShort))
	return api, clock
}

// serveToolCallRequest serves a request without test assertions, so it
// can run in a goroutine.
func serveToolCallRequest(ctx context.Context, handler http.Handler, method, path string, body []byte, headers http.Header) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
	for k, v := range headers {
		r.Header[k] = v
	}
	handler.ServeHTTP(w, r)
	return w
}

func toolCallHeaders(chatID uuid.UUID, messageID int64, toolCallID string, age time.Duration) http.Header {
	h := http.Header{workspacesdk.CoderChatIDHeader: {chatID.String()}}
	workspacesdk.ToolCall{MessageID: messageID, ID: toolCallID, Age: age}.SetHeaders(h)
	return h
}

func postCancel(t *testing.T, handler http.Handler, id string, headers http.Header) *httptest.ResponseRecorder {
	t.Helper()
	return postCancelContext(testutil.Context(t, testutil.WaitLong), handler, id, headers)
}

func postCancelContext(ctx context.Context, handler http.Handler, id string, headers http.Header) *httptest.ResponseRecorder {
	return serveToolCallRequest(ctx, handler, http.MethodPost, fmt.Sprintf("/%s/cancel", id), nil, headers)
}

// requireCancel cancels the tool call's process and requires a 200.
func requireCancel(t *testing.T, handler http.Handler, chatID uuid.UUID, messageID int64, toolCallID string, age time.Duration) workspacesdk.CancelProcessResponse {
	t.Helper()

	id := workspacesdk.ToolCallUUID(chatID, messageID, toolCallID).String()
	w := postCancel(t, handler, id, toolCallHeaders(chatID, messageID, toolCallID, age))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp workspacesdk.CancelProcessResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

func decodeToolCallError(t *testing.T, w *httptest.ResponseRecorder) workspacesdk.ToolCallError {
	t.Helper()

	var resp workspacesdk.ToolCallError
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

func listIDs(t *testing.T, handler http.Handler) []string {
	t.Helper()

	w := getList(t, handler)
	require.Equal(t, http.StatusOK, w.Code)
	var resp workspacesdk.ListProcessesResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	ids := make([]string, 0, len(resp.Processes))
	for _, p := range resp.Processes {
		ids = append(ids, p.ID)
	}
	return ids
}
