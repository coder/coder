package agentacp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestRecovery(t *testing.T) {
	t.Parallel()
	for _, mode := range []fakeHarnessMode{fakeHarnessModeBoth, fakeHarnessModeLoad, fakeHarnessModeResume, fakeHarnessModeNone} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			m, dir, _ := newTestManager(t, mode)

			info := createTestSession(t, m, dir)
			ctx := testutil.Context(t, testutil.WaitLong)
			_, err := m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hello"})
			require.NoError(t, err)
			before, err := awaitTestSession(ctx, m, info.ID)
			require.NoError(t, err)
			// A dead harness with retained history prefers resume and keeps its epoch.
			_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "exit"})
			require.NoError(t, err)
			_, err = awaitTestSession(ctx, m, info.ID)
			require.NoError(t, err)
			read, err := m.Read(ctx, info.ID, nil)
			restored := read.Session
			if mode == fakeHarnessModeNone {
				require.ErrorIs(t, err, ErrUnavailable)
				return
			}
			require.NoError(t, err)
			require.Equal(t, workspacesdk.ACPSessionStatusIdle, restored.Status)
			if mode == fakeHarnessModeLoad {
				require.NotEqual(t, before.Session.Cursor.Epoch, restored.Cursor.Epoch)
				_, err = m.Read(context.Background(), info.ID, &before.Session.Cursor)
				require.ErrorIs(t, err, ErrConflict)
			} else {
				require.Equal(t, before.Session.Cursor.Epoch, restored.Cursor.Epoch)
			}
			require.NoError(t, m.Close())
			// A new agent has no transcript. Load is preferred; resume is incomplete.
			fresh := NewManager(context.Background(), Options{
				Logger:     m.logger,
				Clock:      m.clock,
				Execer:     m.execer,
				Filesystem: m.fs,
				EnvInfo:    m.envInfo,
				WorkingDir: func() string { return dir },
			})
			t.Cleanup(func() { require.NoError(t, fresh.Close()) })
			require.NoError(t, fresh.Reload(ctx))
			read, err = fresh.Read(ctx, info.ID, nil)
			restored = read.Session
			require.NoError(t, err)
			require.Equal(t, mode != fakeHarnessModeResume, restored.HistoryComplete)
			result, err := awaitTestSession(ctx, fresh, info.ID)
			require.NoError(t, err)
			if mode != fakeHarnessModeResume {
				require.Equal(t, "answer:hello:other", result.AssistantResponse)
			} else {
				require.Empty(t, result.AssistantResponse)
			}
			_, err = fresh.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "restored"})
			require.NoError(t, err)
			result, err = awaitTestSession(ctx, fresh, info.ID)
			require.NoError(t, err)
			require.Equal(t, "answer:restored:other", result.AssistantResponse)
			// A missing native file must produce an error instead of a replacement.
			require.NoError(t, os.Remove(filepath.Join(dir, info.ID.SessionID+".json")))
			fresh = restartTestManager(t, fresh, dir)
			_, err = fresh.Read(ctx, info.ID, nil)
			require.ErrorIs(t, err, ErrUnavailable)
			_, err = awaitTestSession(ctx, fresh, info.ID)
			require.ErrorIs(t, err, ErrUnavailable)
		})
	}
}

func TestCreateDeduplication(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)

	req := workspacesdk.ACPCreateSessionRequest{RequestID: uuid.New(), HarnessSlug: "fake", WorkingDirectory: dir}
	first, err := m.Create(context.Background(), req)
	require.NoError(t, err)
	req.Config = map[string]string{}
	second, err := m.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	req.Config["model"] = "other"
	_, err = m.Create(context.Background(), req)
	require.ErrorIs(t, err, ErrConflict)
	req.Config = nil
	second, err = m.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
}

func restartTestManager(t *testing.T, m *Manager, dir string) *Manager {
	t.Helper()
	require.NoError(t, m.Close())
	fresh := NewManager(context.Background(), Options{
		Logger: m.logger, Clock: m.clock, Execer: m.execer,
		Filesystem: afero.NewReadOnlyFs(m.fs), EnvInfo: m.envInfo,
		WorkingDir: func() string { return dir },
	})
	t.Cleanup(func() { require.NoError(t, fresh.Close()) })
	require.NoError(t, fresh.Reload(testutil.Context(t, testutil.WaitLong)))
	return fresh
}

func TestRecoveryThroughSessionOperations(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"read", "message", "interrupt", "subscribe"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			m, dir, _ := newTestManager(t, fakeHarnessModeBoth)

			info := createTestSession(t, m, dir)
			ctx := testutil.Context(t, testutil.WaitLong)
			_, err := m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hello"})
			require.NoError(t, err)
			_, err = awaitTestSession(ctx, m, info.ID)
			require.NoError(t, err)
			// ACP-owned files remain, but all agent-owned metadata is gone.
			fresh := restartTestManager(t, m, dir)
			require.Empty(t, fresh.List())
			switch operation {
			case "read":
				_, err = fresh.Read(ctx, info.ID, nil)
			case "message":
				_, err = fresh.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "after restart"})
			case "interrupt":
				_, err = fresh.Interrupt(ctx, info.ID)
			case "subscribe":
				var unsubscribe func()
				_, _, unsubscribe, err = fresh.Subscribe(ctx, info.ID, nil)
				if unsubscribe != nil {
					unsubscribe()
				}
			}
			require.NoError(t, err)
			result, err := awaitTestSession(ctx, fresh, info.ID)
			require.NoError(t, err)
			require.Equal(t, info.ID, result.Session.ID)
			require.True(t, result.Session.HistoryComplete)
			require.NotEqual(t, info.Cursor.Epoch, result.Session.Cursor.Epoch)
			want := "answer:hello:other"
			if operation == "message" {
				want = "answer:after restart:other"
			}
			require.Equal(t, want, result.AssistantResponse)
			require.Len(t, fresh.List(), 1)
		})
	}
}

func TestConcurrentRecovery(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)

	info := createTestSession(t, m, dir)
	fresh := restartTestManager(t, m, dir)
	ctx := testutil.Context(t, testutil.WaitLong)
	type result struct {
		info workspacesdk.ACPSessionResponse
		err  error
	}
	results := make(chan result, 10)
	for range 10 {
		go func() {
			info, err := fresh.Read(ctx, info.ID, nil)
			results <- result{info: info, err: err}
		}()
	}
	var epoch uuid.UUID
	for range 10 {
		select {
		case result := <-results:
			require.NoError(t, result.err)
			require.Equal(t, info.ID, result.info.Session.ID)
			if epoch == uuid.Nil {
				epoch = result.info.Session.Cursor.Epoch
			}
			require.Equal(t, epoch, result.info.Session.Cursor.Epoch)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	require.Len(t, fresh.List(), 1)
}

func TestMessageRecoversWithoutReplayingInterruptedWork(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)

	info := createTestSession(t, m, dir)
	ctx := testutil.Context(t, testutil.WaitLong)
	crash := workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "exit"}
	_, err := m.Message(ctx, info.ID, crash)
	require.NoError(t, err)
	failed, err := awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusError, failed.Session.Status)
	// Retrying the admitted message does not submit it to the recovered process.
	_, err = m.Message(ctx, info.ID, crash)
	require.NoError(t, err)
	idle, err := awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusIdle, idle.Session.Status)
	_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "recovered"})
	require.NoError(t, err)
	result, err := awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	require.Equal(t, "answer:recovered:other", result.AssistantResponse)
	raw, err := os.ReadFile(filepath.Join(dir, info.ID.SessionID+".json"))
	require.NoError(t, err)
	var native fakeNativeSession
	require.NoError(t, json.Unmarshal(raw, &native))
	require.Equal(t, []string{"recovered", "answer:recovered:other"}, native.History)
}

func TestRecoveryAvailability(t *testing.T) {
	t.Parallel()
	m, dir, configs := newTestManager(t, fakeHarnessModeBoth)

	info := createTestSession(t, m, dir)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "exit"})
	require.NoError(t, err)
	failed, err := awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusError, failed.Session.Status)
	require.NoError(t, os.Remove(filepath.Join(configs, "fake.json")))
	require.NoError(t, m.Reload(ctx))
	_, err = m.Read(ctx, info.ID, nil)
	require.ErrorIs(t, err, ErrUnavailable)
	writeConfig(t, filepath.Join(configs, "fake.json"), map[string]any{"command": fakeCommand(t, fakeHarnessModeBoth)})
	require.NoError(t, m.Reload(ctx))
	restored, err := m.Read(ctx, info.ID, nil)
	require.NoError(t, err)
	require.Equal(t, info.ID, restored.Session.ID)
}

func TestSessionIdentityTuple(t *testing.T) {
	t.Parallel()
	m, dir, configs := newTestManager(t, fakeHarnessModeBoth)
	ctx := testutil.Context(t, testutil.WaitLong)

	info := createTestSession(t, m, dir)
	otherDir := t.TempDir()
	raw, err := os.ReadFile(filepath.Join(dir, info.ID.SessionID+".json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(otherDir, info.ID.SessionID+".json"), raw, 0o600))
	writeConfig(t, filepath.Join(configs, "other.json"), map[string]any{"command": fakeCommand(t, fakeHarnessModeBoth)})
	require.NoError(t, m.Reload(ctx))
	for _, id := range []workspacesdk.ACPSessionID{
		info.ID,
		{HarnessSlug: "other", WorkingDirectory: dir, SessionID: info.ID.SessionID},
		{HarnessSlug: "fake", WorkingDirectory: otherDir, SessionID: info.ID.SessionID},
	} {
		read, err := m.Read(ctx, id, nil)
		require.NoError(t, err)
		require.Equal(t, id, read.Session.ID)
	}
	require.Len(t, m.List(), 3)
	missing := info.ID
	missing.SessionID = "missing"
	_, err = m.Read(ctx, missing, nil)
	require.ErrorIs(t, err, ErrUnavailable)
	require.NoFileExists(t, filepath.Join(dir, "missing.json"))
	for _, invalid := range []workspacesdk.ACPSessionID{
		{HarnessSlug: "fake", WorkingDirectory: dir},
		{WorkingDirectory: dir, SessionID: info.ID.SessionID},
		{HarnessSlug: "fake", SessionID: info.ID.SessionID},
	} {
		_, err := m.Read(ctx, invalid, nil)
		require.ErrorIs(t, err, ErrInvalid)
	}
}
