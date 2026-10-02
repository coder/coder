package agentacp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

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

func TestRecovery(t *testing.T) {
	t.Parallel()
	for _, mode := range []fakeHarnessMode{fakeHarnessModeBoth, fakeHarnessModeLoad, fakeHarnessModeResume, fakeHarnessModeNone} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			m, dir, _ := newTestManager(t, mode)
			info := createTestSession(t, m, dir)
			messageTestSession(t, m, info.ID, "hello")
			before := messageTestSession(t, m, info.ID, "latest")
			ctx := testutil.Context(t, testutil.WaitLong)
			if mode == fakeHarnessModeNone {
				failed := messageTestSession(t, m, info.ID, "exit")
				require.Equal(t, workspacesdk.ACPSessionStatusError, failed.Session.Status)
				read, err := m.Read(ctx, info.ID, nil)
				require.NoError(t, err)
				require.Equal(t, failed, read)
				_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{Text: "unavailable"})
				require.ErrorIs(t, err, ErrUnavailable)
				return
			}
			// Only native files survive restart. Prefer load to resume.
			m = restartTestManager(t, m, dir)
			restored, err := m.Read(ctx, info.ID, nil)
			require.NoError(t, err)
			require.Equal(t, info.ID, restored.Session.ID)
			require.NotEqual(t, before.Session.Cursor.GenerationID, restored.Session.Cursor.GenerationID)
			if mode == fakeHarnessModeResume {
				require.Empty(t, restored.AssistantResponse)
			} else {
				require.Equal(t, before.AssistantResponse, restored.AssistantResponse)
				require.Len(t, restored.Events, 1)
				require.Equal(t, workspacesdk.ACPEventKindUpdate, restored.Events[0].Kind)
				require.Contains(t, string(restored.Events[0].Update), before.AssistantResponse)
			}
			_, err = m.Read(ctx, info.ID, &before.Session.Cursor)
			require.ErrorIs(t, err, ErrConflict)
			require.Equal(t, "answer:restored:other", messageTestSession(t, m, info.ID, "restored").AssistantResponse)
		})
	}
}

func TestMessageRecoveryDoesNotReplayWork(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
	info := createTestSession(t, m, dir)
	ctx := testutil.Context(t, testutil.WaitLong)
	var history []string
	for _, failure := range []string{"crash", "write"} {
		s, err := m.get(ctx, info.ID)
		require.NoError(t, err)
		message := workspacesdk.ACPMessageRequest{Text: failure}
		if failure == "crash" {
			messageTestSession(t, m, info.ID, "exit")
		} else {
			s.proc.stdin = failingWriter{s.proc.stdin}
			_, err = m.Message(ctx, info.ID, message)
			require.ErrorContains(t, err, "broken harness input")
		}
		failed, err := s.Read(ctx, nil)
		require.NoError(t, err)
		require.Equal(t, workspacesdk.ACPSessionStatusError, failed.Session.Status)
		_, err = m.Message(ctx, info.ID, message)
		require.NoError(t, err)
		result := awaitTestSession(t, m, info.ID)
		require.Equal(t, "answer:"+failure+":other", result.AssistantResponse)
		raw, err := os.ReadFile(filepath.Join(dir, info.ID.SessionID+".json"))
		require.NoError(t, err)
		var native fakeNativeSession
		require.NoError(t, json.Unmarshal(raw, &native))
		history = append(history, failure, result.AssistantResponse)
		require.Equal(t, history, native.History)
	}
}

func TestFailedRecoveryPreservesTranscript(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"missing", "partial", "unconfigured"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			m, dir, configs := newTestManager(t, fakeHarnessModeBoth)
			info := createTestSession(t, m, dir)
			ctx := testutil.Context(t, testutil.WaitLong)
			s := holdTestSession(t, m, info.ID)
			s.proc.cancel()
			before := awaitTestSession(t, m, info.ID)
			require.Equal(t, workspacesdk.ACPSessionStatusError, before.Session.Status)
			require.Equal(t, "started", before.AssistantResponse)
			_, events, err := m.Watch(ctx, info.ID, nil)
			require.NoError(t, err)
			nativeFile := filepath.Join(dir, info.ID.SessionID+".json")
			configFile := filepath.Join(configs, "fake.json")
			switch failure {
			case "missing":
				require.NoError(t, os.Remove(nativeFile))
			case "partial":
				writeConfig(t, nativeFile, fakeNativeSession{Value: "fail-load"})
			case "unconfigured":
				require.NoError(t, os.Remove(configFile))
				require.NoError(t, m.Reload(ctx))
			}
			_, err = m.recover(ctx, info.ID)
			require.ErrorIs(t, err, ErrUnavailable)
			after, err := m.Read(ctx, info.ID, nil)
			require.NoError(t, err)
			require.Equal(t, before, after, "failed restoration must not publish partial replay or reset the generation")
			select {
			case event, ok := <-events:
				t.Fatalf("failed recovery changed the subscription: event=%#v open=%t", event, ok)
			default:
			}
			switch failure {
			case "missing":
				require.NoFileExists(t, nativeFile)
			case "unconfigured":
				writeConfig(t, configFile, map[string]any{"command": fakeCommand(t, fakeHarnessModeBoth)})
				require.NoError(t, m.Reload(ctx))
				require.Equal(t, "answer:retry:other", messageTestSession(t, m, info.ID, "retry").AssistantResponse)
			}
		})
	}
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
}

type initializationWaitContext struct {
	context.Context
	waiting chan<- struct{}
}

func (c initializationWaitContext) Done() <-chan struct{} {
	c.waiting <- struct{}{}
	return c.Context.Done()
}

func TestConcurrentRecovery(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		mode     fakeHarnessMode
		existing bool
	}{
		{name: "recover", mode: fakeHarnessModeBoth, existing: true},
		{name: "recover_failure", mode: fakeHarnessModeHang, existing: true},
		{name: "load", mode: fakeHarnessModeBoth},
		{name: "load_failure", mode: fakeHarnessModeHang},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
			info := createTestSession(t, m, dir)
			failed := messageTestSession(t, m, info.ID, "exit")
			ctx := testutil.Context(t, testutil.WaitLong)
			old, err := m.get(ctx, info.ID)
			require.NoError(t, err)
			if !tc.existing {
				m = restartTestManager(t, m, dir)
			}
			setTestHarnessCommand(m, fakeCommand(t, tc.mode))
			clock := m.clock.(*quartz.Mock)
			trap := clock.Trap().AfterFunc("acp-timeout")
			caller, cancel := context.WithCancel(ctx)
			defer cancel()
			canceled := make(chan error, 1)
			go func() { _, err := m.recover(caller, info.ID); canceled <- err }()
			call := trap.MustWait(ctx)
			get := m.recover
			var events <-chan workspacesdk.ACPEvent
			if tc.existing {
				read, stream, err := m.Watch(ctx, info.ID, nil)
				require.NoError(t, err)
				require.Equal(t, failed, read)
				events = stream
			} else {
				get = m.get
			}
			type result struct {
				session *SessionFSM
				err     error
			}
			results, waiting := make(chan result, 10), make(chan struct{}, 10)
			waitCtx := initializationWaitContext{Context: ctx, waiting: waiting}
			for range 10 {
				go func() { s, err := get(waitCtx, info.ID); results <- result{s, err} }()
			}
			for range 10 {
				testutil.RequireReceive(ctx, t, waiting)
			}
			cancel()
			require.ErrorIs(t, testutil.RequireReceive(ctx, t, canceled), context.Canceled)
			call.MustRelease(ctx)
			trap.Close()
			if tc.mode == fakeHarnessModeHang {
				clock.Advance(sessionInitializationTimeout).MustWait(ctx)
			}
			first := testutil.RequireReceive(ctx, t, results)
			for range 9 {
				other := testutil.RequireReceive(ctx, t, results)
				if tc.mode == fakeHarnessModeHang {
					require.Same(t, first.err, other.err)
				} else {
					require.NoError(t, other.err)
					require.Same(t, first.session, other.session)
				}
			}
			if tc.mode == fakeHarnessModeHang {
				require.ErrorIs(t, first.err, ErrUnavailable)
				setTestHarnessCommand(m, fakeCommand(t, fakeHarnessModeBoth))
				first.session, first.err = m.recover(ctx, info.ID)
			}
			require.NoError(t, first.err)
			require.NotSame(t, old, first.session)
			current, err := m.recover(ctx, info.ID)
			require.NoError(t, err)
			require.Same(t, first.session, current, "healthy sessions must not be replaced")
			if tc.existing {
				notification := sessionNotification(info.ID, "stale")
				require.ErrorIs(t, (&sessionClient{session: old}).SessionUpdate(ctx, notification), ErrUnavailable)
				_, err := old.Read(ctx, nil)
				require.ErrorIs(t, err, ErrUnavailable)
				requireClosed(ctx, t, events)
			}
		})
	}
}
