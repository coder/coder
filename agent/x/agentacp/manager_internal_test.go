package agentacp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	require.NoError(t, m.Reload(testutil.Context(t, testutil.WaitLong)))
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

func newTransportSession(t *testing.T, status workspacesdk.ACPSessionStatus) (*Manager, *SessionFSM) {
	t.Helper()
	s, _, _ := newRuntimeSession(t, status)
	m := NewManager(context.Background(), Options{Clock: s.proc.clock, Logger: slogtest.Make(t, nil)})
	m.sessions[s.id], m.instances = s, []*SessionFSM{s}
	s.run = m.run
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	return m, s
}

func createTestSession(t *testing.T, m *Manager, dir string) workspacesdk.ACPSession {
	t.Helper()
	info, err := m.Create(context.Background(), workspacesdk.ACPCreateSessionRequest{HarnessSlug: "fake", WorkingDirectory: dir, Config: map[string]string{"model": "other"}})
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
	caller, cancel := context.WithCancel(ctx)
	result, err := m.Message(caller, info.ID, message)
	require.NoError(t, err)
	cancel()
	require.Equal(t, workspacesdk.ACPMessageOutcomePrompt, result.Outcome)
	wait := awaitTestSession(t, m, info.ID)
	require.Equal(t, "answer:hello:other", wait.AssistantResponse)
	require.Equal(t, workspacesdk.ACPSessionStatusIdle, wait.Session.Status)
	require.Equal(t, workspacesdk.ACPEventKindUserMessage, wait.Events[0].Kind)
	var updates []string
	for _, event := range wait.Events {
		updates = append(updates, string(event.Update))
	}
	require.Contains(t, strings.Join(updates, "\n"), "thinking")
	require.Equal(t, workspacesdk.ACPEventKindStatus, wait.Events[len(wait.Events)-1].Kind)
	require.Len(t, m.List(), 1)
	s := holdTestSession(t, m, info.ID)
	_, err = m.Interrupt(ctx, info.ID)
	require.NoError(t, err)
	wait = awaitTestSession(t, m, info.ID)
	require.Equal(t, string(acp.StopReasonCancelled), wait.Session.StopReason)
	require.NoError(t, m.Close())
	select {
	case <-s.proc.done:
	default:
		t.Fatal("harness was not reaped")
	}
	require.NoError(t, m.Close())
}

func TestRequiredDirectoryAndOverrides(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
	file := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	for _, directory := range []string{"", ".", "relative", filepath.Join(dir, "absent"), file} {
		req := workspacesdk.ACPCreateSessionRequest{HarnessSlug: "fake", WorkingDirectory: directory}
		_, err := m.Create(context.Background(), req)
		require.ErrorIs(t, err, ErrInvalid)
		_, err = m.Read(context.Background(), workspacesdk.ACPSessionID{HarnessSlug: req.HarnessSlug, WorkingDirectory: directory, SessionID: "native"}, nil)
		require.ErrorIs(t, err, ErrInvalid)
	}
	_, err := m.Create(context.Background(), workspacesdk.ACPCreateSessionRequest{HarnessSlug: "fake", WorkingDirectory: dir, Config: map[string]string{"model": "unknown"}})
	require.ErrorIs(t, err, ErrInvalid)
}

func awaitTestSession(t *testing.T, m *Manager, id workspacesdk.ACPSessionID) workspacesdk.ACPSessionResponse {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	s, err := m.get(ctx, id)
	require.NoError(t, err)
	result, err := s.Wait(ctx)
	if result.Session.Status != workspacesdk.ACPSessionStatusError {
		require.NoError(t, err)
	}
	return result
}

func holdTestSession(t *testing.T, m *Manager, id workspacesdk.ACPSessionID) *SessionFSM {
	t.Helper()
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	defer cancel()
	s, err := m.get(ctx, id)
	require.NoError(t, err)
	_, events, err := s.Watch(ctx, nil)
	require.NoError(t, err)
	_, err = m.Message(ctx, id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hold"})
	require.NoError(t, err)
	for {
		if event := testutil.RequireReceive(ctx, t, events); strings.Contains(string(event.Update), `"started"`) {
			return s
		}
	}
}

func messageTestSession(t *testing.T, m *Manager, id workspacesdk.ACPSessionID, text string) workspacesdk.ACPSessionResponse {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := m.Message(ctx, id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: text})
	require.NoError(t, err)
	return awaitTestSession(t, m, id)
}

func setTestHarnessCommand(m *Manager, command string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.configs["fake"]
	cfg.command = command
	m.configs["fake"] = cfg
}
