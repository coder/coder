package agentproc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentgit"
	"github.com/coder/coder/v2/agent/agentproc"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

// postStart sends a POST /start request and returns the recorder.
func postStart(t *testing.T, handler http.Handler, req workspacesdk.StartProcessRequest, headers ...http.Header) *httptest.ResponseRecorder {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	body, err := json.Marshal(req)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/start", bytes.NewReader(body))
	for _, h := range headers {
		for k, vals := range h {
			for _, v := range vals {
				r.Header.Add(k, v)
			}
		}
	}
	handler.ServeHTTP(w, r)
	return w
}

// getList sends a GET /list request and returns the recorder.
func getList(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/list", nil)
	handler.ServeHTTP(w, r)
	return w
}

// getOutput sends a GET /{id}/output request and returns the
// recorder.
func getOutput(t *testing.T, handler http.Handler, id string) *httptest.ResponseRecorder {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/%s/output", id), nil)
	handler.ServeHTTP(w, r)
	return w
}

// postSignal sends a POST /{id}/signal request and returns
// the recorder.
func postSignal(t *testing.T, handler http.Handler, id string, req workspacesdk.SignalProcessRequest) *httptest.ResponseRecorder {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	body, err := json.Marshal(req)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("/%s/signal", id), bytes.NewReader(body))
	handler.ServeHTTP(w, r)
	return w
}

// newTestAPI creates a new API with a test logger and default
// execer, returning the handler and API.
func newTestAPI(t *testing.T) http.Handler {
	t.Helper()
	return newTestAPIWithUpdateEnv(t, nil)
}

// newTestAPIWithUpdateEnv creates a new API with an optional
// updateEnv hook for testing environment injection.
func newTestAPIWithUpdateEnv(t *testing.T, updateEnv func([]string) ([]string, error)) http.Handler {
	t.Helper()

	logger := slogtest.Make(t, &slogtest.Options{
		IgnoreErrors: true,
	}).Leveled(slog.LevelDebug)
	api := agentproc.NewAPI(logger, agentexec.DefaultExecer, updateEnv, nil)
	t.Cleanup(func() {
		_ = api.Close()
	})
	return api.Routes()
}

// waitForExit polls the output endpoint until the process is
// no longer running or the context expires.
func waitForExit(t *testing.T, handler http.Handler, id string) workspacesdk.ProcessOutputResponse {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for process to exit")
		case <-ticker.C:
			w := getOutput(t, handler, id)
			require.Equal(t, http.StatusOK, w.Code)

			var resp workspacesdk.ProcessOutputResponse
			err := json.NewDecoder(w.Body).Decode(&resp)
			require.NoError(t, err)

			if !resp.Running {
				return resp
			}
		}
	}
}

// startAndGetID is a helper that starts a process and returns
// the process ID.
func startAndGetID(t *testing.T, handler http.Handler, req workspacesdk.StartProcessRequest, headers ...http.Header) string {
	t.Helper()

	w := postStart(t, handler, req, headers...)
	require.Equal(t, http.StatusOK, w.Code)

	var resp workspacesdk.StartProcessResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	require.True(t, resp.Started)
	require.NotEmpty(t, resp.ID)
	return resp.ID
}

func TestStartProcess(t *testing.T) {
	t.Parallel()

	t.Run("ForegroundCommand", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)
		w := postStart(t, handler, workspacesdk.StartProcessRequest{
			Command: "echo hello",
		})
		require.Equal(t, http.StatusOK, w.Code)

		var resp workspacesdk.StartProcessResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.True(t, resp.Started)
		require.NotEmpty(t, resp.ID)
	})

	t.Run("BackgroundCommand", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)
		w := postStart(t, handler, workspacesdk.StartProcessRequest{
			Command:    "echo background",
			Background: true,
		})
		require.Equal(t, http.StatusOK, w.Code)

		var resp workspacesdk.StartProcessResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.True(t, resp.Started)
		require.NotEmpty(t, resp.ID)
	})

	t.Run("EmptyCommand", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)
		w := postStart(t, handler, workspacesdk.StartProcessRequest{
			Command: "",
		})
		require.Equal(t, http.StatusBadRequest, w.Code)

		var resp codersdk.Response
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.Contains(t, resp.Message, "Command is required")
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		w := httptest.NewRecorder()
		r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/start", strings.NewReader("{invalid json"))
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusBadRequest, w.Code)

		var resp codersdk.Response
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.Contains(t, resp.Message, "valid JSON")
	})

	t.Run("CustomWorkDir", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)
		tmpDir := t.TempDir()

		// Write a marker file to verify the command ran in
		// the correct directory. Comparing pwd output is
		// unreliable on Windows where Git Bash returns POSIX
		// paths.
		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: "touch marker.txt && ls marker.txt",
			WorkDir: tmpDir,
		})

		resp := waitForExit(t, handler, id)
		require.NotNil(t, resp.ExitCode)
		require.Equal(t, 0, *resp.ExitCode)
		require.Contains(t, resp.Output, "marker.txt")
	})

	t.Run("CustomEnv", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		// Use a unique env var name to avoid collisions in
		// parallel tests.
		envKey := fmt.Sprintf("TEST_PROC_ENV_%d", time.Now().UnixNano())
		envVal := "custom_value_12345"

		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: fmt.Sprintf("printenv %s", envKey),
			Env:     map[string]string{envKey: envVal},
		})

		resp := waitForExit(t, handler, id)
		require.NotNil(t, resp.ExitCode)
		require.Equal(t, 0, *resp.ExitCode)
		require.Contains(t, strings.TrimSpace(resp.Output), envVal)
	})

	t.Run("UpdateEnvHook", func(t *testing.T) {
		t.Parallel()

		envKey := fmt.Sprintf("TEST_UPDATE_ENV_%d", time.Now().UnixNano())
		envVal := "injected_by_hook"

		handler := newTestAPIWithUpdateEnv(t, func(current []string) ([]string, error) {
			return append(current, fmt.Sprintf("%s=%s", envKey, envVal)), nil
		})

		// The process should see the variable even though it
		// was not passed in req.Env.
		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: fmt.Sprintf("printenv %s", envKey),
		})

		resp := waitForExit(t, handler, id)
		require.NotNil(t, resp.ExitCode)
		require.Equal(t, 0, *resp.ExitCode)
		require.Contains(t, strings.TrimSpace(resp.Output), envVal)
	})

	t.Run("UpdateEnvHookOverriddenByReqEnv", func(t *testing.T) {
		t.Parallel()

		envKey := fmt.Sprintf("TEST_OVERRIDE_%d", time.Now().UnixNano())
		hookVal := "from_hook"
		reqVal := "from_request"

		handler := newTestAPIWithUpdateEnv(t, func(current []string) ([]string, error) {
			return append(current, fmt.Sprintf("%s=%s", envKey, hookVal)), nil
		})

		// req.Env should take precedence over the hook.
		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: fmt.Sprintf("printenv %s", envKey),
			Env:     map[string]string{envKey: reqVal},
		})

		resp := waitForExit(t, handler, id)
		require.NotNil(t, resp.ExitCode)
		require.Equal(t, 0, *resp.ExitCode)
		// When duplicate env vars exist, shells use the last
		// value. Since req.Env is appended after the hook,
		// the request value wins.
		require.Contains(t, strings.TrimSpace(resp.Output), reqVal)
	})
}

func TestListProcesses(t *testing.T) {
	t.Parallel()

	t.Run("NoProcesses", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)
		w := getList(t, handler)
		require.Equal(t, http.StatusOK, w.Code)

		var resp workspacesdk.ListProcessesResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Processes)
		require.Empty(t, resp.Processes)
	})

	t.Run("FilterByChatID", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		chatA := uuid.New().String()
		chatB := uuid.New().String()
		headersA := http.Header{workspacesdk.CoderChatIDHeader: {chatA}}
		headersB := http.Header{workspacesdk.CoderChatIDHeader: {chatB}}

		// Start processes with different chat IDs.
		id1 := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: "echo chat-a",
		}, headersA)
		waitForExit(t, handler, id1)

		id2 := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: "echo chat-b",
		}, headersB)
		waitForExit(t, handler, id2)

		id3 := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: "echo chat-a-2",
		}, headersA)
		waitForExit(t, handler, id3)

		// List with chat A header should return 2 processes.
		w := getListWithChatHeader(t, handler, chatA)
		require.Equal(t, http.StatusOK, w.Code)

		var resp workspacesdk.ListProcessesResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.Len(t, resp.Processes, 2)

		ids := make(map[string]bool)
		for _, p := range resp.Processes {
			ids[p.ID] = true
		}
		require.True(t, ids[id1])
		require.True(t, ids[id3])

		// List with chat B header should return 1 process.
		w2 := getListWithChatHeader(t, handler, chatB)
		require.Equal(t, http.StatusOK, w2.Code)

		var resp2 workspacesdk.ListProcessesResponse
		err = json.NewDecoder(w2.Body).Decode(&resp2)
		require.NoError(t, err)
		require.Len(t, resp2.Processes, 1)
		require.Equal(t, id2, resp2.Processes[0].ID)

		// List without chat header should return all 3.
		w3 := getList(t, handler)
		require.Equal(t, http.StatusOK, w3.Code)

		var resp3 workspacesdk.ListProcessesResponse
		err = json.NewDecoder(w3.Body).Decode(&resp3)
		require.NoError(t, err)
		require.Len(t, resp3.Processes, 3)
	})

	t.Run("ChatIDFiltering", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)
		chatID := uuid.New().String()
		headers := http.Header{workspacesdk.CoderChatIDHeader: {chatID}}

		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: "echo with-chat",
		}, headers)
		waitForExit(t, handler, id)

		// Listing with the same chat header should return
		// the process.
		w := getListWithChatHeader(t, handler, chatID)
		require.Equal(t, http.StatusOK, w.Code)

		var resp workspacesdk.ListProcessesResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.Len(t, resp.Processes, 1)
		require.Equal(t, id, resp.Processes[0].ID)

		// Listing with a different chat header should not
		// return the process.
		w2 := getListWithChatHeader(t, handler, uuid.New().String())
		require.Equal(t, http.StatusOK, w2.Code)

		var resp2 workspacesdk.ListProcessesResponse
		err = json.NewDecoder(w2.Body).Decode(&resp2)
		require.NoError(t, err)
		require.Empty(t, resp2.Processes)

		// Listing without a chat header should return the
		// process (no filtering).
		w3 := getList(t, handler)
		require.Equal(t, http.StatusOK, w3.Code)

		var resp3 workspacesdk.ListProcessesResponse
		err = json.NewDecoder(w3.Body).Decode(&resp3)
		require.NoError(t, err)
		require.Len(t, resp3.Processes, 1)
	})

	t.Run("SortAndLimit", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		// Start 12 short-lived processes so we exceed the
		// limit of 10.
		for i := 0; i < 12; i++ {
			id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
				Command: fmt.Sprintf("echo proc-%d", i),
			})
			waitForExit(t, handler, id)
		}

		w := getList(t, handler)
		require.Equal(t, http.StatusOK, w.Code)

		var resp workspacesdk.ListProcessesResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.Len(t, resp.Processes, 10, "should be capped at 10")

		// All returned processes are exited, so they should
		// be sorted by StartedAt descending (newest first).
		for i := 1; i < len(resp.Processes); i++ {
			require.GreaterOrEqual(t, resp.Processes[i-1].StartedAt, resp.Processes[i].StartedAt,
				"processes should be sorted by started_at descending")
		}
	})

	t.Run("RunningProcessesSortedFirst", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		// Start an exited process first.
		exitedID := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: "echo done",
		})
		waitForExit(t, handler, exitedID)

		// Start a running process after.
		runningID := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command:    "sleep 300",
			Background: true,
		})

		w := getList(t, handler)
		require.Equal(t, http.StatusOK, w.Code)

		var resp workspacesdk.ListProcessesResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.Len(t, resp.Processes, 2)

		// Running process should come first regardless of
		// start order.
		require.Equal(t, runningID, resp.Processes[0].ID)
		require.True(t, resp.Processes[0].Running)
		require.Equal(t, exitedID, resp.Processes[1].ID)
		require.False(t, resp.Processes[1].Running)

		// Clean up.
		postSignal(t, handler, runningID, workspacesdk.SignalProcessRequest{
			Signal: "kill",
		})
	})

	t.Run("MixedRunningAndExited", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		// Start a process that exits quickly.
		exitedID := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: "echo done",
		})
		waitForExit(t, handler, exitedID)

		// Start a long-running process.
		runningID := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command:    "sleep 300",
			Background: true,
		})

		// List should contain both.
		w := getList(t, handler)
		require.Equal(t, http.StatusOK, w.Code)

		var resp workspacesdk.ListProcessesResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.Len(t, resp.Processes, 2)

		procMap := make(map[string]workspacesdk.ProcessInfo)
		for _, p := range resp.Processes {
			procMap[p.ID] = p
		}

		exited, ok := procMap[exitedID]
		require.True(t, ok, "exited process should be in list")
		require.False(t, exited.Running)
		require.NotNil(t, exited.ExitCode)

		running, ok := procMap[runningID]
		require.True(t, ok, "running process should be in list")
		require.True(t, running.Running)

		// Clean up the long-running process.
		sw := postSignal(t, handler, runningID, workspacesdk.SignalProcessRequest{
			Signal: "kill",
		})
		require.Equal(t, http.StatusOK, sw.Code)
	})
}

// getListWithChatHeader sends a GET /list request with the
// Coder-Chat-Id header set and returns the recorder.
func getListWithChatHeader(t *testing.T, handler http.Handler, chatID string) *httptest.ResponseRecorder {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/list", nil)
	if chatID != "" {
		r.Header.Set(workspacesdk.CoderChatIDHeader, chatID)
	}
	handler.ServeHTTP(w, r)
	return w
}

func TestProcessOutput(t *testing.T) {
	t.Parallel()

	t.Run("ExitedProcess", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: "echo hello-output",
		})

		resp := waitForExit(t, handler, id)
		require.False(t, resp.Running)
		require.NotNil(t, resp.ExitCode)
		require.Equal(t, 0, *resp.ExitCode)
		require.Contains(t, resp.Output, "hello-output")
	})

	t.Run("RunningProcess", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command:    "sleep 300",
			Background: true,
		})

		w := getOutput(t, handler, id)
		require.Equal(t, http.StatusOK, w.Code)

		var resp workspacesdk.ProcessOutputResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.True(t, resp.Running)

		// Kill and wait for the process so cleanup does
		// not hang.
		postSignal(
			t, handler, id,
			workspacesdk.SignalProcessRequest{Signal: "kill"},
		)
		waitForExit(t, handler, id)
	})

	t.Run("NonexistentProcess", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)
		w := getOutput(t, handler, "nonexistent-id-12345")
		require.Equal(t, http.StatusNotFound, w.Code)

		var resp codersdk.Response
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.Contains(t, resp.Message, "not found")
	})
}

func TestSignalProcess(t *testing.T) {
	t.Parallel()

	t.Run("KillRunning", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command:    "sleep 300",
			Background: true,
		})

		w := postSignal(t, handler, id, workspacesdk.SignalProcessRequest{
			Signal: "kill",
		})
		require.Equal(t, http.StatusOK, w.Code)

		// Verify the process exits.
		resp := waitForExit(t, handler, id)
		require.False(t, resp.Running)
	})

	t.Run("TerminateRunning", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("SIGTERM is not supported on Windows")
		}

		handler := newTestAPI(t)

		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command:    "sleep 300",
			Background: true,
		})

		w := postSignal(t, handler, id, workspacesdk.SignalProcessRequest{
			Signal: "terminate",
		})
		require.Equal(t, http.StatusOK, w.Code)

		// Verify the process exits.
		resp := waitForExit(t, handler, id)
		require.False(t, resp.Running)
	})

	t.Run("NonexistentProcess", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)
		w := postSignal(t, handler, "nonexistent-id-12345", workspacesdk.SignalProcessRequest{
			Signal: "kill",
		})
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("AlreadyExitedProcess", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: "echo done",
		})

		// Wait for exit first.
		waitForExit(t, handler, id)

		// Signaling an exited process should return 409
		// Conflict via the errProcessNotRunning sentinel.
		w := postSignal(t, handler, id, workspacesdk.SignalProcessRequest{
			Signal: "kill",
		})
		assert.Equal(t, http.StatusConflict, w.Code,
			"expected 409 for signaling exited process, got %d", w.Code)
	})

	t.Run("EmptySignal", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command:    "sleep 300",
			Background: true,
		})

		w := postSignal(t, handler, id, workspacesdk.SignalProcessRequest{
			Signal: "",
		})
		require.Equal(t, http.StatusBadRequest, w.Code)

		var resp codersdk.Response
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.Contains(t, resp.Message, "Signal is required")

		// Clean up.
		postSignal(t, handler, id, workspacesdk.SignalProcessRequest{
			Signal: "kill",
		})
	})

	t.Run("InvalidSignal", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command:    "sleep 300",
			Background: true,
		})

		w := postSignal(t, handler, id, workspacesdk.SignalProcessRequest{
			Signal: "SIGFOO",
		})
		require.Equal(t, http.StatusBadRequest, w.Code)

		var resp codersdk.Response
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.Contains(t, resp.Message, "Unsupported signal")

		// Clean up.
		postSignal(t, handler, id, workspacesdk.SignalProcessRequest{
			Signal: "kill",
		})
	})
}

func TestHandleStartProcess_ChatHeaders_EmptyWorkDir_StillNotifies(t *testing.T) {
	t.Parallel()

	pathStore := agentgit.NewPathStore()
	chatID := uuid.New()
	ch, unsub := pathStore.Subscribe(chatID)
	defer unsub()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	api := agentproc.NewAPI(logger, agentexec.DefaultExecer, func(current []string) ([]string, error) {
		return current, nil
	}, pathStore)
	defer api.Close()

	routes := api.Routes()

	body, err := json.Marshal(workspacesdk.StartProcessRequest{
		Command: "echo hello",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/start", bytes.NewReader(body))
	req.Header.Set(workspacesdk.CoderChatIDHeader, chatID.String())
	rw := httptest.NewRecorder()
	routes.ServeHTTP(rw, req)

	require.Equal(t, http.StatusOK, rw.Code)

	// The subscriber should be notified even though no paths
	// were added.
	select {
	case <-ch:
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out waiting for path store notification")
	}

	// No paths should have been stored for this chat.
	require.Nil(t, pathStore.GetPaths(chatID))
}

func TestProcessLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("StartWaitCheckOutput", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: "echo lifecycle-test && echo second-line",
		})

		resp := waitForExit(t, handler, id)
		require.False(t, resp.Running)
		require.NotNil(t, resp.ExitCode)
		require.Equal(t, 0, *resp.ExitCode)
		require.Contains(t, resp.Output, "lifecycle-test")
		require.Contains(t, resp.Output, "second-line")
	})

	t.Run("NonZeroExitCode", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: "exit 42",
		})

		resp := waitForExit(t, handler, id)
		require.False(t, resp.Running)
		require.NotNil(t, resp.ExitCode)
		require.Equal(t, 42, *resp.ExitCode)
	})

	t.Run("StartSignalVerifyExit", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		// Start a long-running background process.
		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command:    "sleep 300",
			Background: true,
		})

		// Verify it's running.
		w := getOutput(t, handler, id)
		require.Equal(t, http.StatusOK, w.Code)
		var running workspacesdk.ProcessOutputResponse
		err := json.NewDecoder(w.Body).Decode(&running)
		require.NoError(t, err)
		require.True(t, running.Running)

		// Signal it.
		sw := postSignal(t, handler, id, workspacesdk.SignalProcessRequest{
			Signal: "kill",
		})
		require.Equal(t, http.StatusOK, sw.Code)

		// Verify it exits.
		resp := waitForExit(t, handler, id)
		require.False(t, resp.Running)
		require.NotNil(t, resp.ExitCode)
	})

	t.Run("OutputExceedsBuffer", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		// Generate output that exceeds MaxHeadBytes +
		// MaxTailBytes. Each line is ~100 chars, and we
		// need more than 32KB total (16KB head + 16KB
		// tail).
		lineCount := (agentproc.MaxHeadBytes+agentproc.MaxTailBytes)/50 + 500
		cmd := fmt.Sprintf(
			"for i in $(seq 1 %d); do echo \"line-$i-padding-to-make-this-longer-than-fifty-characters-total\"; done",
			lineCount,
		)

		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: cmd,
		})

		resp := waitForExit(t, handler, id)
		require.False(t, resp.Running)
		require.NotNil(t, resp.ExitCode)
		require.Equal(t, 0, *resp.ExitCode)

		// The output should be truncated with head/tail
		// strategy metadata.
		require.NotNil(t, resp.Truncated, "large output should be truncated")
		require.Equal(t, "head_tail", resp.Truncated.Strategy)
		require.Greater(t, resp.Truncated.OmittedBytes, 0)
		require.Greater(t, resp.Truncated.OriginalBytes, resp.Truncated.RetainedBytes)

		// Verify the output contains the omission marker.
		require.Contains(t, resp.Output, "... [omitted")
	})

	t.Run("StderrCaptured", func(t *testing.T) {
		t.Parallel()

		handler := newTestAPI(t)

		id := startAndGetID(t, handler, workspacesdk.StartProcessRequest{
			Command: "echo stdout-msg && echo stderr-msg >&2",
		})

		resp := waitForExit(t, handler, id)
		require.False(t, resp.Running)
		require.NotNil(t, resp.ExitCode)
		require.Equal(t, 0, *resp.ExitCode)
		// Both stdout and stderr should be captured.
		require.Contains(t, resp.Output, "stdout-msg")
		require.Contains(t, resp.Output, "stderr-msg")
	})
}
