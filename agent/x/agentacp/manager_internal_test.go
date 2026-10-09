package agentacp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type testEnv struct {
	usershell.SystemEnvInfo
	home string
}

func (e testEnv) HomeDir() (string, error) { return e.home, nil }

func newTestManager(t *testing.T, mode fakeHarnessMode) (manager *Manager, directory, configsDirectory string) {
	t.Helper()
	home, dir := t.TempDir(), t.TempDir()
	m := NewManager(context.Background(), Options{
		Logger:     slogtest.Make(t, nil),
		Clock:      quartz.NewMock(t),
		Execer:     agentexec.DefaultExecer,
		Filesystem: afero.NewOsFs(),
		EnvInfo:    testEnv{home: home},
		WorkingDir: func() string { return dir },
	})
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	configDir := filepath.Join(home, ".coder", "acp")
	require.NoError(t, os.MkdirAll(configDir, 0o700))
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": fakeCommand(t, mode)})
	require.NoError(t, m.Reload(context.Background()))
	require.Len(t, m.Catalog(), 1)
	require.Empty(t, m.Catalog()[0].Error)
	return m, dir, configDir
}

func writeConfig(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
}

func createTestSession(t *testing.T, m *Manager, dir string) workspacesdk.ACPSession {
	t.Helper()
	info, err := m.Create(context.Background(), workspacesdk.ACPCreateSessionRequest{RequestID: uuid.New(), HarnessSlug: "fake", WorkingDirectory: dir, Config: map[string]string{"model": "other"}})
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusIdle, info.Status)
	return info
}

func TestSessionLifecycle(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)

	info := createTestSession(t, m, dir)
	ctx := testutil.Context(t, testutil.WaitLong)
	message := workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hello"}
	result, err := m.Message(ctx, info.ID, message)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPMessageOutcomePrompt, result.Outcome)
	duplicate, err := m.Message(ctx, info.ID, message)
	require.NoError(t, err)
	require.Equal(t, result, duplicate)
	message.Text = "different"
	_, err = m.Message(ctx, info.ID, message)
	require.ErrorIs(t, err, ErrConflict)
	wait, err := awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	require.Equal(t, "answer:hello:other", wait.AssistantResponse)
	require.Equal(t, workspacesdk.ACPSessionStatusIdle, wait.Session.Status)
	require.Equal(t, workspacesdk.ACPEventKindUserMessage, wait.Events[0].Kind)
	require.Contains(t, string(wait.Events[2].Update), "thinking")
	require.Equal(t, workspacesdk.ACPEventKindStatus, wait.Events[len(wait.Events)-1].Kind)
	require.Len(t, m.List(), 1)
	baseline, events, unsubscribe, err := m.Subscribe(context.Background(), info.ID, &wait.Session.Cursor)
	require.NoError(t, err)
	defer unsubscribe()
	require.Len(t, baseline, 1)
	_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hold"})
	require.NoError(t, err)
	// A notification establishes that the harness is inside the active prompt.
	for event := range events {
		if string(event.Update) != "" && event.Kind == workspacesdk.ACPEventKindUpdate && string(event.Update) != "null" {
			break
		}
	}
	for _, text := range []string{"steer", "new"} {
		admission, err := m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: text})
		require.NoError(t, err)
		want := workspacesdk.ACPMessageOutcomeInjected
		if text == "new" {
			want = workspacesdk.ACPMessageOutcomeStartedNewTurn
		}
		require.Equal(t, want, admission.Outcome)
	}
	_, err = m.Interrupt(ctx, info.ID)
	require.NoError(t, err)
	wait, err = awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	require.Equal(t, string(acp.StopReasonCancelled), wait.Session.StopReason)
	_, err = m.Interrupt(ctx, info.ID)
	require.NoError(t, err)
	require.NoError(t, m.Close())
}

func TestRequiredDirectoryAndOverrides(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
	file := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	for _, directory := range []string{"", ".", "relative", filepath.Join(dir, "absent"), file} {
		req := workspacesdk.ACPCreateSessionRequest{RequestID: uuid.New(), HarnessSlug: "fake", WorkingDirectory: directory}
		_, err := m.Create(context.Background(), req)
		require.ErrorIs(t, err, ErrInvalid)
		_, err = m.Read(context.Background(), workspacesdk.ACPSessionID{HarnessSlug: req.HarnessSlug, WorkingDirectory: directory, SessionID: "native"}, nil)
		require.ErrorIs(t, err, ErrInvalid)
	}
	_, err := m.Create(context.Background(), workspacesdk.ACPCreateSessionRequest{RequestID: uuid.New(), HarnessSlug: "fake", WorkingDirectory: dir, Config: map[string]string{"model": "unknown"}})
	require.ErrorIs(t, err, ErrInvalid)
	info := createTestSession(t, m, dir)
	require.NotEmpty(t, info.ID.SessionID)
}

func TestPermissionsAndCallbacks(t *testing.T) {
	t.Parallel()
	c := &client{}
	require.NoError(t, c.SessionUpdate(context.Background(), acp.SessionNotification{}))
	for _, test := range []struct {
		options  []acp.PermissionOption
		canceled bool
		want     string
	}{
		{options: []acp.PermissionOption{{OptionId: "once", Kind: acp.PermissionOptionKindAllowOnce}, {OptionId: "always", Kind: acp.PermissionOptionKindAllowAlways}}, want: "always"},
		{options: []acp.PermissionOption{{OptionId: "once", Kind: acp.PermissionOptionKindAllowOnce}}, want: "once"},
		{options: []acp.PermissionOption{{OptionId: "deny", Kind: acp.PermissionOptionKindRejectAlways}}},
		{options: []acp.PermissionOption{{OptionId: "always", Kind: acp.PermissionOptionKindAllowAlways}}, canceled: true},
	} {
		ctx, cancel := context.WithCancel(context.Background())
		if test.canceled {
			cancel()
		}
		result, err := c.RequestPermission(ctx, acp.RequestPermissionRequest{Options: test.options})
		cancel()
		require.NoError(t, err)
		if test.want == "" {
			require.NotNil(t, result.Outcome.Cancelled) //nolint:misspell // ACP uses this spelling.
		} else {
			require.Equal(t, test.want, string(result.Outcome.Selected.OptionId))
		}
	}
	_, err := c.ReadTextFile(context.Background(), acp.ReadTextFileRequest{})
	require.Error(t, err)
	_, err = c.WriteTextFile(context.Background(), acp.WriteTextFileRequest{})
	require.Error(t, err)
	_, err = c.CreateTerminal(context.Background(), acp.CreateTerminalRequest{})
	require.Error(t, err)
	_, err = c.KillTerminal(context.Background(), acp.KillTerminalRequest{})
	require.Error(t, err)
	_, err = c.TerminalOutput(context.Background(), acp.TerminalOutputRequest{})
	require.Error(t, err)
	_, err = c.ReleaseTerminal(context.Background(), acp.ReleaseTerminalRequest{})
	require.Error(t, err)
	_, err = c.WaitForTerminalExit(context.Background(), acp.WaitForTerminalExitRequest{})
	require.Error(t, err)
}

func TestSubscriberCancellationAndSlowSubscriber(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)

	info := createTestSession(t, m, dir)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hold"})
	require.NoError(t, err)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = awaitTestSession(canceled, m, info.ID)
	require.ErrorIs(t, err, context.Canceled)
	_, events, unsubscribe, err := m.Subscribe(context.Background(), info.ID, nil)
	require.NoError(t, err)
	defer unsubscribe()
	s, err := m.get(ctx, info.ID)
	require.NoError(t, err)
	s.mu.Lock()
	for range 100 {
		s.appendLocked(workspacesdk.ACPEventKindUpdate, "", nil)
	}
	s.mu.Unlock()
	count := 0
	for range events {
		count++
	}
	require.Equal(t, 64, count)
	read, err := m.Read(context.Background(), info.ID, nil)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusRunning, read.Session.Status)
	_, err = m.Interrupt(ctx, info.ID)
	require.NoError(t, err)
	stopped, err := awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	require.Equal(t, string(acp.StopReasonCancelled), stopped.Session.StopReason)
	require.NoError(t, m.Close())
}

func TestConcurrentSessionsAndProcessFailure(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeNone)

	ctx := testutil.Context(t, testutil.WaitLong)
	type outcome struct {
		response workspacesdk.ACPSessionResponse
		err      error
	}
	outcomes := make(chan outcome, 3)
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			info, err := m.Create(ctx, workspacesdk.ACPCreateSessionRequest{RequestID: uuid.New(), HarnessSlug: "fake", WorkingDirectory: dir, Config: map[string]string{"model": "other"}})
			if err == nil {
				_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hello"})
			}
			var result workspacesdk.ACPSessionResponse
			if err == nil {
				result, err = awaitTestSession(ctx, m, info.ID)
			}
			outcomes <- outcome{response: result, err: err}
		})
	}
	wg.Wait()
	close(outcomes)
	for result := range outcomes {
		require.NoError(t, result.err)
		require.Equal(t, "answer:hello:other", result.response.AssistantResponse)
	}
	info := createTestSession(t, m, dir)
	_, err := m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hold"})
	require.NoError(t, err)
	_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "steer"})
	require.ErrorIs(t, err, ErrConflict)
	_, err = m.Interrupt(ctx, info.ID)
	require.NoError(t, err)
	_, err = awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "exit"})
	require.NoError(t, err)
	result, err := awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusError, result.Session.Status)
	require.NotEmpty(t, result.Session.Error)
	require.False(t, errors.Is(err, ErrInvalid))
}

func TestAcceptedWorkAndShutdown(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)

	// Canceling the caller cannot cancel accepted setup or prompts.
	req := workspacesdk.ACPCreateSessionRequest{RequestID: uuid.New(), HarnessSlug: "fake", WorkingDirectory: dir}
	caller, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := m.Create(caller, req)
	require.ErrorIs(t, err, context.Canceled)
	ctx := testutil.Context(t, testutil.WaitLong)
	info, err := m.Create(ctx, req)
	require.NoError(t, err)
	caller, cancel = context.WithCancel(ctx)
	_, err = m.Message(caller, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hello"})
	require.NoError(t, err)
	cancel()
	result, err := awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	require.Equal(t, "answer:hello:default", result.AssistantResponse)
	s, err := m.get(ctx, info.ID)
	require.NoError(t, err)
	s.mu.Lock()
	p := s.proc
	s.mu.Unlock()
	require.NoError(t, m.Close())
	select {
	case <-p.done:
	default:
		t.Fatal("harness was not reaped")
	}
	require.NoError(t, m.Close())
}

func TestReadConcurrentWithShutdown(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)

	ctx := testutil.Context(t, testutil.WaitLong)
	info := createTestSession(t, m, dir)
	_, err := m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hello"})
	require.NoError(t, err)
	before, err := awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	var wg sync.WaitGroup
	errorsCh := make(chan error, 10)
	for range 10 {
		wg.Go(func() {
			_, readErr := m.Read(context.Background(), info.ID, &before.Session.Cursor)
			errorsCh <- readErr
		})
	}
	require.NoError(t, m.Close())
	wg.Wait()
	close(errorsCh)
	for readErr := range errorsCh {
		if readErr != nil {
			require.True(t, errors.Is(readErr, ErrNotFound) || errors.Is(readErr, ErrUnavailable), "%v", readErr)
		}
	}
}

func awaitTestSession(ctx context.Context, m *Manager, id workspacesdk.ACPSessionID) (workspacesdk.ACPSessionResponse, error) {
	baseline, events, unsubscribe, err := m.Subscribe(ctx, id, nil)
	if err != nil {
		return workspacesdk.ACPSessionResponse{}, err
	}
	defer unsubscribe()
	info := baseline[len(baseline)-1].Session
	for info.Status == workspacesdk.ACPSessionStatusRunning || info.Status == workspacesdk.ACPSessionStatusStarting {
		select {
		case <-ctx.Done():
			return workspacesdk.ACPSessionResponse{}, ctx.Err()
		case event, ok := <-events:
			if !ok {
				return workspacesdk.ACPSessionResponse{}, ErrUnavailable
			}
			if event.Session != nil {
				info = event.Session
			}
		}
	}
	s, err := m.get(ctx, id)
	if err != nil {
		return workspacesdk.ACPSessionResponse{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	start := uint64(0)
	if s.lastUser != 0 {
		start = s.lastUser - 1
	}
	return s.readLocked(nil, start)
}
