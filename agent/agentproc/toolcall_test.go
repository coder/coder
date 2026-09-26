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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentgit"
	"github.com/coder/coder/v2/agent/agentproc"
	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

const (
	// longRunning is how long the agent has been running in tests that
	// need every tool call to be provably new to the agent.
	longRunning = time.Hour
	// stopAny is a stop threshold above the run age of every process in
	// these tests, which do not advance the clock that far.
	stopAny = 10 * time.Minute
)

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
				resp := requireCancel(t, handler, chatID, 1, "call", 0, stopAny)
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

	first := postStart(t, handler, req, toolCallHeaders(chatID, 1, "call", 0))
	require.Equal(t, http.StatusInternalServerError, first.Code, first.Body.String())
	again := postStart(t, handler, req, toolCallHeaders(chatID, 1, "call", 0))
	require.Equal(t, http.StatusInternalServerError, again.Code)
	assert.Equal(t, first.Body.String(), again.Body.String(), "the failed start is recorded, not retried")

	resp := requireCancel(t, handler, chatID, 1, "call", 0, stopAny)
	assert.True(t, resp.Started)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Nil(t, resp.Process, "a failed start has no process")
}

func TestStartProcessWithoutToolCallUnchanged(t *testing.T) {
	t.Parallel()

	handler, _ := newToolCallTestAPI(t, longRunning)
	headers := http.Header{workspacesdk.CoderChatIDHeader: {uuid.New().String()}}
	req := workspacesdk.StartProcessRequest{Command: "true"}

	first := postStart(t, handler, req, headers)
	require.Equal(t, http.StatusOK, first.Code)
	assert.Empty(t, first.Header().Get(workspacesdk.CoderToolCallRunAgeMsHeader))
	second := startAndGetID(t, handler, req, headers)
	var resp workspacesdk.StartProcessResponse
	require.NoError(t, json.NewDecoder(first.Body).Decode(&resp))
	require.NotEqual(t, resp.ID, second, "starts without tool call headers are independent")
}

func TestStartProcessToolCallBadRequest(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	withChat := func(h http.Header) http.Header {
		h.Set(workspacesdk.CoderChatIDHeader, chatID.String())
		return h
	}
	toolCallOnly := func() http.Header {
		h := http.Header{}
		workspacesdk.ToolCall{MessageID: 1, ID: "call"}.SetHeaders(h)
		return h
	}
	malformedAge := toolCallOnly()
	malformedAge.Set(workspacesdk.CoderToolCallAgeMsHeader, "-1")

	tests := []struct {
		name    string
		headers http.Header
	}{
		{name: "WithoutChat", headers: toolCallOnly()},
		{name: "PartialHeaders", headers: withChat(http.Header{workspacesdk.CoderToolCallMessageIDHeader: {"1"}})},
		{name: "MalformedAge", headers: withChat(malformedAge)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newToolCallTestAPI(t, longRunning)
			w := postStart(t, handler, workspacesdk.StartProcessRequest{Command: "true"}, tt.headers)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			// Nothing was recorded, so a valid request still starts.
			startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "true"}, toolCallHeaders(chatID, 1, "call", 0))
		})
	}
}

func TestCancelToolCallProcess(t *testing.T) {
	t.Parallel()

	t.Run("Running", func(t *testing.T) {
		t.Parallel()

		handler, clock := newToolCallTestAPI(t, longRunning)
		chatID := uuid.New()
		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "echo before; sleep 300"}, toolCallHeaders(chatID, 1, "call", 0))
		waitForOutput(t, handler, id, "before")
		clock.Advance(2 * time.Second).MustWait(testutil.Context(t, testutil.WaitShort))

		resp := requireCancel(t, handler, chatID, 1, "call", 0, stopAny)
		assert.True(t, resp.Started)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(resp.Body), id, "the recorded start response")
		require.NotNil(t, resp.Process)
		assert.False(t, resp.Process.Running)
		assert.True(t, resp.Process.Canceled)
		assert.Equal(t, "before\n", resp.Process.Output)
		require.NotNil(t, resp.Process.ExitCode)
		assert.NotZero(t, *resp.Process.ExitCode)
		assert.EqualValues(t, 2000, resp.Process.RunAgeMs)

		out := waitForExit(t, handler, id)
		assert.Equal(t, resp.Process.ExitCode, out.ExitCode)
	})

	// An interrupt task retry sends the cancel again after the process
	// exited from the first one.
	t.Run("RepeatedCancel", func(t *testing.T) {
		t.Parallel()

		handler, _ := newToolCallTestAPI(t, longRunning)
		chatID := uuid.New()
		startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "echo before; sleep 300"}, toolCallHeaders(chatID, 1, "call", 0))

		first := requireCancel(t, handler, chatID, 1, "call", 0, stopAny)
		second := requireCancel(t, handler, chatID, 1, "call", 0, stopAny)
		require.NotNil(t, first.Process)
		require.NotNil(t, second.Process)
		assert.True(t, first.Process.Canceled)
		assert.True(t, second.Process.Canceled, "a repeated cancel must still report that the user canceled the process")
		assert.Equal(t, first, second)
	})

	t.Run("Exited", func(t *testing.T) {
		t.Parallel()

		handler, _ := newToolCallTestAPI(t, longRunning)
		chatID := uuid.New()
		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "echo done; exit 3"}, toolCallHeaders(chatID, 1, "call", 0))
		waitForExit(t, handler, id)

		resp := requireCancel(t, handler, chatID, 1, "call", 0, stopAny)
		assert.True(t, resp.Started)
		require.NotNil(t, resp.Process)
		assert.False(t, resp.Process.Canceled)
		assert.Equal(t, "done\n", resp.Process.Output)
		require.NotNil(t, resp.Process.ExitCode)
		assert.Equal(t, 3, *resp.Process.ExitCode)
	})

	t.Run("AbortedRequestStillKills", func(t *testing.T) {
		t.Parallel()

		handler, _ := newToolCallTestAPI(t, longRunning)
		chatID := uuid.New()
		headers := toolCallHeaders(chatID, 1, "call", 0)
		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "sleep 300"}, headers)

		aborted, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		cancel()
		postCancelContext(aborted, handler, id, headers, stopAny)
		out := waitForExit(t, handler, id)
		assert.NotNil(t, out.ExitCode)
		// The retried cancel reports the kill the aborted one sent.
		resp := requireCancel(t, handler, chatID, 1, "call", 0, stopAny)
		require.NotNil(t, resp.Process)
		assert.True(t, resp.Process.Canceled)
	})

	// A kill through process_signal is the model's own action, not a user
	// cancel.
	t.Run("SignalKillIsNotCanceled", func(t *testing.T) {
		t.Parallel()

		handler, _ := newToolCallTestAPI(t, longRunning)
		chatID := uuid.New()
		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "sleep 300"}, toolCallHeaders(chatID, 1, "call", 0))
		w := postSignal(t, handler, id, workspacesdk.SignalProcessRequest{Signal: "kill"})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		waitForExit(t, handler, id)

		resp := requireCancel(t, handler, chatID, 1, "call", 0, stopAny)
		require.NotNil(t, resp.Process)
		assert.False(t, resp.Process.Running)
		assert.False(t, resp.Process.Canceled)
	})

	t.Run("NeverReceived", func(t *testing.T) {
		t.Parallel()

		handler, _ := newToolCallTestAPI(t, longRunning)
		chatID := uuid.New()
		for range 2 {
			resp := requireCancel(t, handler, chatID, 1, "call", 0, stopAny)
			assert.Equal(t, workspacesdk.CancelToolCallResponse{}, resp)
		}
	})

	t.Run("AgentStartedAfterToolCall", func(t *testing.T) {
		t.Parallel()

		handler, _ := newToolCallTestAPI(t, 10*time.Second)
		chatID := uuid.New()
		w := postCancel(t, handler, workspacesdk.ToolCallUUID(chatID, 1, "call").String(), toolCallHeaders(chatID, 1, "call", time.Minute), stopAny)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		require.Equal(t, workspacesdk.ToolCallErrorAgentStartedAfterToolCall, decodeToolCallError(t, w).Code)
	})

	t.Run("StaleMessage", func(t *testing.T) {
		t.Parallel()

		handler, _ := newToolCallTestAPI(t, longRunning)
		chatID := uuid.New()
		startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "true"}, toolCallHeaders(chatID, 2, "other", 0))
		w := postCancel(t, handler, workspacesdk.ToolCallUUID(chatID, 1, "call").String(), toolCallHeaders(chatID, 1, "call", 0), stopAny)
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
		w := postCancel(t, handler, idA, toolCallHeaders(chatB, 1, "call", 0), stopAny)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		// The same tool call identifiers in chat B are a different tool call.
		resp := requireCancel(t, handler, chatB, 1, "call", 0, stopAny)
		assert.False(t, resp.Started)

		assert.True(t, requireOutput(t, handler, idA).Running, "chat A's process must keep running")
	})
}

func TestCancelToolCallProcessRunAgeRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		runAge     time.Duration
		stopBelow  time.Duration
		wantKilled bool
	}{
		{name: "BelowThresholdKills", runAge: 4 * time.Second, stopBelow: 5 * time.Second, wantKilled: true},
		{name: "AtThresholdLeavesRunning", runAge: 5 * time.Second, stopBelow: 5 * time.Second},
		{name: "AboveThresholdLeavesRunning", runAge: 6 * time.Second, stopBelow: 5 * time.Second},
		{name: "ZeroNeverStops", stopBelow: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, clock := newToolCallTestAPI(t, longRunning)
			chatID := uuid.New()
			id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "sleep 300"}, toolCallHeaders(chatID, 1, "call", 0))
			t.Cleanup(func() {
				postSignal(t, handler, id, workspacesdk.SignalProcessRequest{Signal: "kill"})
			})
			clock.Advance(tt.runAge).MustWait(testutil.Context(t, testutil.WaitShort))

			resp := requireCancel(t, handler, chatID, 1, "call", 0, tt.stopBelow)
			require.NotNil(t, resp.Process)
			assert.Equal(t, !tt.wantKilled, resp.Process.Running)
			assert.Equal(t, tt.wantKilled, resp.Process.Canceled)
			assert.Equal(t, tt.runAge.Milliseconds(), resp.Process.RunAgeMs)
			assert.Equal(t, !tt.wantKilled, requireOutput(t, handler, id).Running)
		})
	}
}

func TestToolCallRunAge(t *testing.T) {
	t.Parallel()

	handler, clock := newToolCallTestAPI(t, longRunning)
	chatID := uuid.New()
	req := workspacesdk.StartProcessRequest{Command: "true"}

	first := postStart(t, handler, req, toolCallHeaders(chatID, 1, "call", 0))
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Equal(t, "0", first.Header().Get(workspacesdk.CoderToolCallRunAgeMsHeader))

	clock.Advance(1500 * time.Millisecond).MustWait(testutil.Context(t, testutil.WaitShort))
	again := postStart(t, handler, req, toolCallHeaders(chatID, 1, "call", 0))
	require.Equal(t, http.StatusOK, again.Code, again.Body.String())
	assert.Equal(t, "1500", again.Header().Get(workspacesdk.CoderToolCallRunAgeMsHeader))
	assert.Equal(t, first.Body.String(), again.Body.String(), "a repeated start gets the recorded body")

	var start workspacesdk.StartProcessResponse
	require.NoError(t, json.NewDecoder(first.Body).Decode(&start))
	waitForExit(t, handler, start.ID)
	resp := requireCancel(t, handler, chatID, 1, "call", 0, 0)
	require.NotNil(t, resp.Process)
	assert.EqualValues(t, 1500, resp.Process.RunAgeMs)
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

func TestCancelToolCallWaitsForPendingStart(t *testing.T) {
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
		_, handler, _ := newToolCallAPI(t, longRunning, nil, updateEnv)
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
			canceled <- postCancelContext(t.Context(), handler, workspacesdk.ToolCallUUID(chatID, 1, "call").String(), headers, stopAny)
		}()
		synctest.Wait()
		require.Empty(t, canceled, "cancel must wait for the pending start")

		close(release)
		w := <-canceled
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp workspacesdk.CancelToolCallResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Started)
		require.NotNil(t, resp.Process)
		assert.True(t, resp.Process.Canceled, "the started process must be killed")
		w = <-started
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})
}

func TestCloseKillsToolCallProcess(t *testing.T) {
	t.Parallel()

	api, handler, _ := newToolCallAPI(t, longRunning, nil, nil)
	id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{Command: "sleep 300"}, toolCallHeaders(uuid.New(), 1, "call", 0))

	require.NoError(t, api.Close())
	resp := requireOutput(t, handler, id)
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
		_, handler, _ := newToolCallAPI(t, longRunning, pathStore, nil)
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

	_, handler, clock := newToolCallAPI(t, uptime, nil, nil)
	return handler, clock
}

// newToolCallAPI returns an API with a tool call store, whose agent has
// been running for uptime on the returned mock clock, and a handler that
// serves the process routes and the cancel route the way the agent mounts
// them. pathStore and updateEnv may be nil.
func newToolCallAPI(t *testing.T, uptime time.Duration, pathStore *agentgit.PathStore, updateEnv func([]string) ([]string, error)) (*agentproc.API, http.Handler, *quartz.Mock) {
	t.Helper()

	clock := quartz.NewMock(t)
	store := agenttoolcall.NewStore(clock)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	api := agentproc.NewAPI(logger, agentexec.DefaultExecer, nil, pathStore, nil, updateEnv, nil, agentproc.WithClock(clock), agentproc.WithToolCallStore(store))
	t.Cleanup(func() {
		_ = api.Close()
	})
	clock.Advance(uptime).MustWait(testutil.Context(t, testutil.WaitShort))

	router := chi.NewRouter()
	router.Post("/tool-calls/{id}/cancel", store.CancelHandler(api))
	router.Mount("/", api.Routes())
	return api, agentchat.Middleware(router), clock
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

func postCancel(t *testing.T, handler http.Handler, id string, headers http.Header, stopBelow time.Duration) *httptest.ResponseRecorder {
	t.Helper()
	return postCancelContext(testutil.Context(t, testutil.WaitLong), handler, id, headers, stopBelow)
}

func postCancelContext(ctx context.Context, handler http.Handler, id string, headers http.Header, stopBelow time.Duration) *httptest.ResponseRecorder {
	body := fmt.Appendf(nil, `{"stop_if_run_age_below_ms":%d}`, stopBelow.Milliseconds())
	return serveToolCallRequest(ctx, handler, http.MethodPost, fmt.Sprintf("/tool-calls/%s/cancel", id), body, headers)
}

// requireCancel cancels the tool call and requires a 200.
func requireCancel(t *testing.T, handler http.Handler, chatID uuid.UUID, messageID int64, toolCallID string, age, stopBelow time.Duration) workspacesdk.CancelToolCallResponse {
	t.Helper()

	id := workspacesdk.ToolCallUUID(chatID, messageID, toolCallID).String()
	w := postCancel(t, handler, id, toolCallHeaders(chatID, messageID, toolCallID, age), stopBelow)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp workspacesdk.CancelToolCallResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

func requireOutput(t *testing.T, handler http.Handler, id string) workspacesdk.ProcessOutputResponse {
	t.Helper()

	w := getOutput(t, handler, id)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp workspacesdk.ProcessOutputResponse
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
