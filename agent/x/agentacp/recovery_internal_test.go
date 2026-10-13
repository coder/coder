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
			// Reads retain the failed instance until new work requests recovery.
			_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "exit"})
			require.NoError(t, err)
			_, err = awaitTestSession(ctx, m, info.ID)
			require.NoError(t, err)
			read, err := m.Read(ctx, info.ID, nil)
			restored := read.Session
			if mode == fakeHarnessModeNone {
				require.NoError(t, err)
				require.Equal(t, workspacesdk.ACPSessionStatusError, read.Session.Status)
				require.Empty(t, read.AssistantResponse)
				_, _, stop, err := m.Subscribe(ctx, info.ID, nil)
				require.NoError(t, err)
				stop()
				_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "unavailable"})
				require.ErrorIs(t, err, ErrUnavailable)
				return
			}
			require.NoError(t, err)
			require.Equal(t, workspacesdk.ACPSessionStatusError, restored.Status)
			recovered, err := m.recover(ctx, info.ID)
			require.NoError(t, err)
			snapshot, err := recovered.Read(ctx, nil)
			require.NoError(t, err)
			require.Equal(t, workspacesdk.ACPSessionStatusIdle, snapshot.Session.Status)
			require.NotEqual(t, before.Session.Cursor.Epoch, snapshot.Session.Cursor.Epoch)
			_, err = m.Read(context.Background(), info.ID, &before.Session.Cursor)
			require.ErrorIs(t, err, ErrConflict)
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
			require.Empty(t, fresh.List())
		})
	}
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
			if operation == "interrupt" {
				require.ErrorIs(t, err, ErrConflict)
			} else {
				require.NoError(t, err)
			}
			result, err := awaitTestSession(ctx, fresh, info.ID)
			require.NoError(t, err)
			require.Equal(t, info.ID, result.Session.ID)
			require.Equal(t, operation != "message", result.Session.HistoryComplete)
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

func TestFailedLoadPreservesHistory(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"missing", "partial"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			m, dir, _ := newTestManager(t, fakeHarnessModeLoad)
			info := createTestSession(t, m, dir)
			ctx := testutil.Context(t, testutil.WaitLong)
			_, err := m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hello"})
			require.NoError(t, err)
			_, err = awaitTestSession(ctx, m, info.ID)
			require.NoError(t, err)
			_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "exit"})
			require.NoError(t, err)
			before, err := awaitTestSession(ctx, m, info.ID)
			require.NoError(t, err)
			nativeFile := filepath.Join(dir, info.ID.SessionID+".json")
			if failure == "missing" {
				require.NoError(t, os.Remove(nativeFile))
			} else {
				raw, err := json.Marshal(fakeNativeSession{Value: "fail-load"})
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(nativeFile, raw, 0o600))
			}
			_, events, unsubscribe, err := m.Subscribe(ctx, info.ID, &before.Session.Cursor)
			require.NoError(t, err)
			defer unsubscribe()
			_, err = m.recover(ctx, info.ID)
			require.ErrorIs(t, err, ErrUnavailable)
			after, err := m.Read(ctx, info.ID, &before.Session.Cursor)
			require.NoError(t, err)
			require.Equal(t, workspacesdk.ACPSessionStatusError, after.Session.Status)
			require.Equal(t, before.Session.Cursor.Epoch, after.Session.Cursor.Epoch)
			for _, event := range after.Events {
				require.NotEqual(t, workspacesdk.ACPEventKindReset, event.Kind)
				require.NotContains(t, string(event.Update), "partial replay")
			}
			select {
			case event := <-events:
				t.Fatalf("unexpected event: %#v", event)
			default:
			}
			read, err := m.Read(ctx, info.ID, nil)
			require.NoError(t, err)
			require.Empty(t, read.AssistantResponse)
			require.NotContains(t, read.AssistantResponse, "partial replay")
		})
	}
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
	observed, err := m.Read(ctx, info.ID, nil)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusError, observed.Session.Status)
	_, _, stop, err := m.Subscribe(ctx, info.ID, nil)
	require.NoError(t, err)
	stop()
	_, err = m.recover(ctx, info.ID)
	require.ErrorIs(t, err, ErrUnavailable)
	writeConfig(t, filepath.Join(configs, "fake.json"), map[string]any{"command": fakeCommand(t, fakeHarnessModeBoth)})
	require.NoError(t, m.Reload(ctx))
	restored, err := m.Read(ctx, info.ID, nil)
	require.NoError(t, err)
	require.Equal(t, info.ID, restored.Session.ID)
	require.Equal(t, workspacesdk.ACPSessionStatusError, restored.Session.Status)
	recovered, err := m.recover(ctx, info.ID)
	require.NoError(t, err)
	restored, err = recovered.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusIdle, restored.Session.Status)
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
