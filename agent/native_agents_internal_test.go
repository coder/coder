package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentscripts"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/agent/x/agentnative"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/pty"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

func TestNativeAgentEnvironment(t *testing.T) {
	t.Parallel()
	env := []string{
		"HOME=/bootstrap", "PATH=/workspace/bin:/usr/bin", "USER=coder",
		"CODER_AGENT_TOKEN=agent-secret", "CODER_AGENT_TOKEN_FILE=/private/token",
		"CODER_SESSION_TOKEN=session-secret", "CODER_NATIVE_JOB_PROMPT_BASE64=prompt",
		"CODER_NATIVE_JOB_CODEX_API_KEY_BASE64=bootstrap-key", "CODER_NATIVE_JOB_REPO_BASE64=repo",
		"OPENAI_API_KEY=native-provider-key", "INVALID",
	}
	original := slices.Clone(env)
	require.ElementsMatch(t, []string{"HOME=/workspace", "PATH=/workspace/bin:/usr/bin", "USER=coder", "OPENAI_API_KEY=native-provider-key"}, nativeAgentEnvironment("/workspace", env))
	require.Equal(t, original, env)
}

func TestNativeAgentCommand(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("native agents require Linux")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "native-test-binary")
	//nolint:gosec // The fixture must be executable to test workspace PATH resolution.
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s' \"$NATIVE_TEST\"\n"), 0o700))
	execer := &nativeRecordingExecer{Execer: agentexec.DefaultExecer}
	env := []string{"PATH=" + dir, "NATIVE_TEST=workspace"}
	command := nativeAgentCommand(execer, dir, env)
	cmd := command(t.Context(), "native-test-binary")
	require.Equal(t, binary, execer.path)
	require.Equal(t, dir, cmd.Dir)
	output, err := cmd.Output()
	require.NoError(t, err)
	require.Equal(t, "workspace", string(output))
	cmd.Env[0] = "PATH=/different"
	require.Equal(t, "PATH="+dir, env[0], "commands must not mutate the shared environment")

	// A real host binary must not be used when absent from the workspace PATH.
	missing := command(t.Context(), "sh")
	require.ErrorIs(t, missing.Err, exec.ErrNotFound)
	require.Error(t, missing.Run())
}

type nativeRecordingExecer struct {
	agentexec.Execer
	path string
}

func (e *nativeRecordingExecer) CommandContext(ctx context.Context, path string, args ...string) *exec.Cmd {
	e.path = path
	return e.Execer.CommandContext(ctx, path, args...)
}

func TestStartNativeAgentsOnce(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("native agents require Linux")
	}
	a, home := newNativeTestAgent(t)
	a.startNativeAgents()
	manager := a.nativeManager.Load()
	require.NotNil(t, manager)
	descriptor, err := agentnative.ReadDescriptor(agentnative.DefaultStateDir(home))
	require.NoError(t, err)
	require.NotEmpty(t, descriptor.Token)

	// A reconnect can update the manifest without constructing another manager.
	a.manifest.Store(&agentsdk.Manifest{Directory: t.TempDir()})
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(a.startNativeAgents)
	}
	wg.Wait()
	require.Same(t, manager, a.nativeManager.Load())
}

func TestStartNativeAgentsDisabledOrClosing(t *testing.T) {
	t.Parallel()
	for _, enabled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		a := &agent{nativeAgents: enabled, gracefulCtx: ctx}
		a.startNativeAgents()
		require.Nil(t, a.nativeManager.Load())
	}
}

func TestStartNativeAgentsClosingBeforePublication(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("native agents require Linux")
	}
	for _, stoppedBeforeStartup := range []bool{false, true} {
		t.Run(strconv.FormatBool(stoppedBeforeStartup), func(t *testing.T) {
			t.Parallel()
			a, home := newNativeTestAgent(t)
			// Model Close winning the publication lock while startup is in flight.
			a.closeMutex.Lock()
			a.closing = true
			a.closeMutex.Unlock()
			if stoppedBeforeStartup {
				a.closeNativeAgents()
			}
			a.startNativeAgents()
			require.Nil(t, a.nativeManager.Load())
			if !stoppedBeforeStartup {
				require.NoError(t, nativeTestLocalShutdown(a.gracefulCtx, home))
				a.closeNativeAgents()
			}
			manager, err := agentnative.New(agentnative.Options{
				StateDir: agentnative.DefaultStateDir(home), Environment: []string{"HOME=" + home},
			})
			require.NoError(t, err, "the unpublished manager must release its exclusive lock after stop scripts")
			require.NoError(t, manager.Close())
		})
	}
}

func TestStartNativeAgentsCloseDuringInitialJob(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("native agents require Linux")
	}
	ctx := testutil.Context(t, testutil.WaitLong)
	started, startupDone := make(chan struct{}), make(chan struct{})
	stopResult := make(chan error, 1)
	execer := &nativeShutdownExecer{Execer: agentexec.DefaultExecer, started: started}
	a, home := newNativeTestAgentWithExecer(t, execer)
	gracefulCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	a.gracefulCtx = gracefulCtx
	a.environmentVariables = map[string]string{"PATH": home + ":/usr/bin:/bin"}
	//nolint:gosec // The fixture must be executable for workspace PATH lookup.
	require.NoError(t, os.WriteFile(filepath.Join(home, "claude"), []byte("#!/bin/sh\nexit 1\n"), 0o700))
	spec := agentnative.JobSpec{Harness: "claude", Cwd: home, Prompt: "initial prompt"}
	stateDir := agentnative.DefaultStateDir(home)
	require.NoError(t, agentnative.WriteConfig(stateDir, agentnative.Config{InitialJob: &spec}))
	execer.onStop = func() {
		// Force startup to finish unwinding before the stop script contacts the
		// local API. Premature observer cleanup would remove that endpoint.
		select {
		case <-startupDone:
			stopResult <- nativeTestLocalShutdown(ctx, home)
		case <-ctx.Done():
			stopResult <- ctx.Err()
		}
	}
	a.scriptRunner.GetScriptLogger = func(uuid.UUID) agentscripts.ScriptLogger { return nativeTestScriptLogger{} }
	require.NoError(t, a.scriptRunner.Init([]codersdk.WorkspaceAgentScript{
		{ID: uuid.New(), LogSourceID: uuid.New(), Script: "true # native-stop", RunOnStop: true},
	}, nil))
	go func() {
		defer close(startupDone)
		a.startNativeAgents()
	}()
	testutil.TryReceive(ctx, t, started)

	// Exercise the Close ordering while RunJob is waiting on a native command.
	a.closeMutex.Lock()
	a.closing = true
	a.closeMutex.Unlock()
	cancel()
	require.NoError(t, a.scriptRunner.Execute(ctx, agentscripts.ExecuteStopScripts))
	require.NoError(t, testutil.TryReceive(ctx, t, stopResult))
	require.Nil(t, a.nativeManager.Load(), "shutdown must not publish the provisional manager")
	a.closeNativeAgents()
	manager, err := agentnative.New(agentnative.Options{StateDir: stateDir, Environment: []string{"HOME=" + home}})
	require.NoError(t, err, "cleanup must release the provisional manager lock")
	require.NoError(t, manager.Close())
}

func TestStartNativeAgentsInvalidConfigStillServes(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("native agents require Linux")
	}
	a, home := newNativeTestAgent(t)
	dir := agentnative.DefaultStateDir(home)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte("invalid config containing a private prompt"), 0o600))
	a.startNativeAgents()
	require.NotNil(t, a.nativeManager.Load(), "configuration failure must not disable existing sessions")
}

func TestStartNativeAgentsInitialJobFailureStillServes(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("native agents require Linux")
	}
	a, home := newNativeTestAgent(t)
	// No native executable is available, so startup must record one failed
	// attempt without preventing access to the manager's session history.
	a.environmentVariables = map[string]string{"PATH": home}
	spec := agentnative.JobSpec{Harness: "claude", Cwd: home, Prompt: "private initial prompt"}
	require.NoError(t, agentnative.WriteConfig(agentnative.DefaultStateDir(home), agentnative.Config{InitialJob: &spec}))
	a.startNativeAgents()
	manager := a.nativeManager.Load()
	require.NotNil(t, manager)
	job, err := manager.RunJob(a.gracefulCtx, spec)
	require.NoError(t, err, "startup must have already recorded the attempt; it must not be replayed")
	require.NotEmpty(t, job.RequestID)
	require.Equal(t, "unknown", job.Status)
}

func TestNativeAgentACPAvailability(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		enabled bool
		status  int
	}{
		{name: "Disabled", status: http.StatusNotFound},
		{name: "NotReady", enabled: true, status: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := &agent{nativeAgents: tc.enabled}
			recorder := httptest.NewRecorder()
			a.handleNativeAgentACP(recorder, httptest.NewRequest(http.MethodGet, "/api/v0/native-agents/acp", nil))
			require.Equal(t, tc.status, recorder.Code)
		})
	}
}

func TestNativeAgentACPReadLimit(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("native agents require Linux")
	}
	a, _ := newNativeTestAgent(t)
	a.startNativeAgents()
	server := httptest.NewServer(http.HandlerFunc(a.handleNativeAgentACP))
	t.Cleanup(server.Close)
	ctx := testutil.Context(t, testutil.WaitLong)
	//nolint:bodyclose // websocket.Dial closes the response body.
	ws, _, err := websocket.Dial(ctx, server.URL, nil)
	require.NoError(t, err)
	defer ws.CloseNow()

	// Every NDJSON line fits the ACP scanner limit. Only the WebSocket
	// message limit can reject this collection of short notifications.
	line := "{\"jsonrpc\":\"2.0\",\"method\":\"ignored\"}\n"
	payload := strings.Repeat(line, (4<<20)/len(line)+1)
	payload += "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":1}}\n"
	written := make(chan struct{})
	go func() {
		defer close(written)
		// Rejection may interrupt the write before the message is complete.
		_ = ws.Write(ctx, websocket.MessageBinary, []byte(payload))
	}()
	_, _, err = ws.Read(ctx)
	require.Equal(t, websocket.StatusMessageTooBig, websocket.CloseStatus(err))
	testutil.TryReceive(ctx, t, written)
}

func TestNativeAgentACPDisconnectPreservesManager(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("native agents require Linux")
	}
	a, home := newNativeTestAgent(t)
	a.startNativeAgents()
	require.NotNil(t, a.nativeManager.Load())
	server := httptest.NewServer(http.HandlerFunc(a.handleNativeAgentACP))
	t.Cleanup(server.Close)
	ctx := testutil.Context(t, testutil.WaitLong)
	for range 2 {
		//nolint:bodyclose // websocket.Dial closes the response body.
		ws, _, err := websocket.Dial(ctx, server.URL, nil)
		require.NoError(t, err)
		request := []byte("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":1}}\n")
		require.NoError(t, ws.Write(ctx, websocket.MessageBinary, request[:17]))
		require.NoError(t, ws.Write(ctx, websocket.MessageBinary, request[17:]))
		var response struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		require.NoError(t, json.NewDecoder(websocket.NetConn(ctx, ws, websocket.MessageBinary)).Decode(&response))
		require.Equal(t, 1, response.ID)
		require.NotEmpty(t, response.Result)
		require.NoError(t, ws.Close(websocket.StatusNormalClosure, "disconnect"))
	}
	descriptor, err := agentnative.ReadDescriptor(agentnative.DefaultStateDir(home))
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, descriptor.URL+"/v1/sessions", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+descriptor.Token)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	_, err = io.Copy(io.Discard, res.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
}

type nativeHomeEnvInfo struct {
	usershell.SystemEnvInfo
	home string
}

func (e nativeHomeEnvInfo) HomeDir() (string, error) { return e.home, nil }
func (e nativeHomeEnvInfo) Environ() []string {
	return []string{"HOME=" + e.home, "PATH=/usr/bin:/bin"}
}

type nativeTokenClient struct{ Client }

func (nativeTokenClient) GetSessionToken() string { return "private-test-agent-token" }

type nativeShutdownExecer struct {
	agentexec.Execer
	started chan struct{}
	onStop  func()
	once    sync.Once
}

func (e *nativeShutdownExecer) CommandContext(ctx context.Context, path string, args ...string) *exec.Cmd {
	if filepath.Base(path) == "claude" {
		e.once.Do(func() { close(e.started) })
		<-ctx.Done()
	}
	return e.Execer.CommandContext(ctx, path, args...)
}

func (e *nativeShutdownExecer) PTYCommandContext(ctx context.Context, path string, args ...string) *pty.Cmd {
	if slices.Contains(args, "true # native-stop") {
		e.onStop()
	}
	return e.Execer.PTYCommandContext(ctx, path, args...)
}

type nativeTestScriptLogger struct{}

func (nativeTestScriptLogger) Send(context.Context, ...agentsdk.Log) error { return nil }
func (nativeTestScriptLogger) Flush(context.Context) error                 { return nil }

func nativeTestLocalShutdown(ctx context.Context, home string) error {
	descriptor, err := agentnative.ReadDescriptor(agentnative.DefaultStateDir(home))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, descriptor.URL+"/v1/shutdown", strings.NewReader("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+descriptor.Token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return xerrors.Errorf("local shutdown returned status %d", res.StatusCode)
	}
	_, err = io.Copy(io.Discard, res.Body)
	return err
}

func newNativeTestAgent(t *testing.T) (*agent, string) {
	t.Helper()
	return newNativeTestAgentWithExecer(t, agentexec.DefaultExecer)
}

func newNativeTestAgentWithExecer(t *testing.T, execer agentexec.Execer) (*agent, string) {
	t.Helper()
	home := t.TempDir()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := testutil.Logger(t)
	a := &agent{
		nativeAgents: true, gracefulCtx: ctx, logger: logger,
		clock: quartz.NewMock(t), envInfo: nativeHomeEnvInfo{home: home},
		execer: execer, client: nativeTokenClient{},
	}
	a.manifest.Store(&agentsdk.Manifest{Directory: home})
	server, err := agentssh.NewServer(ctx, logger, prometheus.NewRegistry(), afero.NewOsFs(), a.execer, &agentssh.Config{
		EnvInfo:          a.envInfo,
		WorkingDirectory: func() string { return a.manifest.Load().Directory },
		UpdateEnv:        a.updateCommandEnv,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })
	a.sshServer = server
	a.scriptRunner = agentscripts.New(agentscripts.Options{Logger: logger, SSHServer: server, Filesystem: afero.NewOsFs(), DataDirBase: t.TempDir(), LogDir: t.TempDir()})
	t.Cleanup(func() { _ = a.scriptRunner.Close() })
	t.Cleanup(a.closeNativeAgents)
	return a, home
}
