package agentproc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func trackedAPI(t *testing.T, updateEnv func([]string) ([]string, error)) (*API, *httptest.Server, *quartz.Mock) {
	t.Helper()
	api := NewAPI(testutil.Logger(t), agentexec.DefaultExecer, nil, nil, nil, updateEnv, nil)
	clock := quartz.NewMock(t)
	api.manager.clock = clock
	t.Cleanup(func() { require.NoError(t, api.Close()) })
	server := httptest.NewServer(agentchat.Middleware(api.Routes()))
	t.Cleanup(server.Close)
	return api, server, clock
}

func processRequest(t *testing.T, server *httptest.Server, path string, input, output any, chatID string) int {
	t.Helper()
	body, err := json.Marshal(input)
	if !assert.NoError(t, err) {
		return 0
	}
	method := http.MethodPost
	if input == nil {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(testutil.Context(t, testutil.WaitLong), method, server.URL+path, bytes.NewReader(body))
	if !assert.NoError(t, err) {
		return 0
	}
	if chatID != "" {
		req.Header.Set(workspacesdk.CoderChatIDHeader, chatID)
	}
	response, err := server.Client().Do(req)
	if !assert.NoError(t, err) {
		return 0
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK && output != nil {
		assert.NoError(t, json.NewDecoder(response.Body).Decode(output))
	}
	return response.StatusCode
}

func waitProcessDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-testutil.Context(t, testutil.WaitLong).Done():
		t.Fatal("process did not finish")
	}
}

func trackedRequest(api *API, command string) workspacesdk.StartProcessRequest {
	req := workspacesdk.StartProcessRequest{Command: command, ProcessID: uuid.New(), AgentInstanceID: api.manager.instanceID}
	req.InputDigest = workspacesdk.StartProcessDigest(req)
	return req
}

func cancelRequest(req workspacesdk.StartProcessRequest) workspacesdk.CancelProcessRequest {
	return workspacesdk.CancelProcessRequest{ProcessID: req.ProcessID, AgentInstanceID: req.AgentInstanceID, InputDigest: req.InputDigest, Deadline: req.Deadline}
}

func TestTrackedProcessFenceDelayedHTTPStart(t *testing.T) {
	t.Parallel()
	reached, release := make(chan struct{}), make(chan struct{})
	releaseStart := sync.OnceFunc(func() { close(release) })
	defer releaseStart()
	api, server, _ := trackedAPI(t, func(env []string) ([]string, error) {
		close(reached)
		<-release
		return env, nil
	})
	req := trackedRequest(api, "echo should-not-run")
	result := make(chan int, 1)
	go func() { result <- processRequest(t, server, "/start-tracked", req, nil, "") }()
	// A real request has passed the first admission check, but has not spawned.
	waitProcessDone(t, reached)
	var response workspacesdk.CancelProcessResponse
	require.Equal(t, http.StatusOK, processRequest(t, server, "/cancel-tracked", cancelRequest(req), &response, ""))
	require.True(t, response.Fenced)
	require.Nil(t, response.Process)
	releaseStart()
	require.Equal(t, http.StatusConflict, testutil.RequireReceive(testutil.Context(t, testutil.WaitLong), t, result))
	require.Empty(t, api.manager.list(uuid.Nil))
	require.Equal(t, http.StatusConflict, processRequest(t, server, "/start-tracked", req, nil, ""))
	changed := cancelRequest(req)
	changed.Deadline = time.Now()
	require.Equal(t, http.StatusConflict, processRequest(t, server, "/cancel-tracked", changed, nil, ""))
}

func TestTrackedProcessWaitDeadlineAndCancellation(t *testing.T) {
	t.Parallel()
	api, server, clock := trackedAPI(t, nil)
	req := trackedRequest(api, "exec sleep 600")
	req.Deadline = clock.Now().Add(time.Hour)
	req.InputDigest = workspacesdk.StartProcessDigest(req)
	require.Equal(t, http.StatusOK, processRequest(t, server, "/start-tracked", req, nil, ""))

	trap := clock.Trap().AfterFunc("process-wait")
	closeTrap := sync.OnceFunc(trap.Close)
	defer closeTrap()
	outputCh := make(chan workspacesdk.ProcessOutputResponse, 1)
	go func() {
		var output workspacesdk.ProcessOutputResponse
		assert.Equal(t, http.StatusOK, processRequest(t, server, "/"+req.ProcessID.String()+"/output?wait=true&wait_ms=1000", nil, &output, ""))
		outputCh <- output
	}()
	ctx := testutil.Context(t, testutil.WaitLong)
	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(time.Second).MustWait(ctx)
	output := testutil.RequireReceive(ctx, t, outputCh)
	require.True(t, output.Running)
	require.Nil(t, output.ExitCode)
	closeTrap()

	clock.Advance(time.Hour - time.Second).MustWait(ctx)
	proc, ok := api.manager.get(uuid.Nil, req.ProcessID.String())
	require.True(t, ok)
	waitProcessDone(t, proc.done)
	require.False(t, proc.info().Running)
	require.NotNil(t, proc.info().ExitCode)
	require.NotEqual(t, 0, *proc.info().ExitCode)

	// No deadline is a supported policy-neutral option. Explicit cancellation
	// still waits for actual exit and returns the observed nonzero exit code.
	req = trackedRequest(api, "exec sleep 600")
	require.Equal(t, http.StatusOK, processRequest(t, server, "/start-tracked", req, nil, ""))
	var canceled workspacesdk.CancelProcessResponse
	cancelReq := cancelRequest(req)
	cancelReq.WaitMillis = 1000
	require.Equal(t, http.StatusOK, processRequest(t, server, "/cancel-tracked", cancelReq, &canceled, ""))
	require.False(t, canceled.Fenced)
	require.NotNil(t, canceled.Process)
	require.False(t, canceled.Process.Running)
	require.NotNil(t, canceled.Process.ExitCode)
	require.NotEqual(t, 0, *canceled.Process.ExitCode)
}

func TestTrackedProcessCancellationPending(t *testing.T) {
	t.Parallel()
	api, server, clock := trackedAPI(t, nil)
	req := trackedRequest(api, "exec sleep 600")
	require.Equal(t, http.StatusOK, processRequest(t, server, "/start-tracked", req, nil, ""))
	proc, ok := api.manager.get(uuid.Nil, req.ProcessID.String())
	require.True(t, ok)
	// Simulate cancellation that has not reached the OS. The response must
	// report the live process instead of inventing a successful exit.
	cancel := proc.cancel
	proc.cancel = func() {}
	defer cancel()
	var immediate workspacesdk.CancelProcessResponse
	require.Equal(t, http.StatusOK, processRequest(t, server, "/cancel-tracked", cancelRequest(req), &immediate, ""))
	require.NotNil(t, immediate.Process)
	require.True(t, immediate.Process.Running)
	require.Nil(t, immediate.Process.ExitCode)
	trap := clock.Trap().AfterFunc("process-wait")
	defer trap.Close()
	result := make(chan workspacesdk.CancelProcessResponse, 1)
	go func() {
		cancelReq := cancelRequest(req)
		cancelReq.WaitMillis = 1000
		var response workspacesdk.CancelProcessResponse
		assert.Equal(t, http.StatusOK, processRequest(t, server, "/cancel-tracked", cancelReq, &response, ""))
		result <- response
	}()
	ctx := testutil.Context(t, testutil.WaitLong)
	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(time.Second).MustWait(ctx)
	response := testutil.RequireReceive(ctx, t, result)
	require.False(t, response.Fenced)
	require.NotNil(t, response.Process)
	require.True(t, response.Process.Running)
	require.Nil(t, response.Process.ExitCode)
}
