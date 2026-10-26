//go:build linux

package agentnative

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

type managerTestBackend struct {
	host      Host
	creates   atomic.Int32
	resumes   atomic.Int32
	prompts   atomic.Int32
	modes     atomic.Int32
	cancels   atomic.Int32
	attaches  atomic.Int32
	closes    atomic.Int32
	shutdowns atomic.Int32
	create    func(context.Context, string) (Session, error)
	resume    func(context.Context, Session) error
	prompt    func(context.Context, Session, Prompt) error
	setMode   func(context.Context, Session, string) error
	shutdown  func(context.Context) error
	attach    func(context.Context, Session) (Command, error)
}

func (b *managerTestBackend) Create(ctx context.Context, cwd string) (Session, error) {
	b.creates.Add(1)
	if b.create != nil {
		return b.create(ctx, cwd)
	}
	id := uuid.NewString()
	return Session{ID: id, NativeID: id, Harness: "claude", Cwd: cwd, Mode: "human"}, nil
}
func (b *managerTestBackend) Resume(ctx context.Context, s Session) error {
	b.resumes.Add(1)
	if b.resume != nil {
		return b.resume(ctx, s)
	}
	_, err := b.host.Session(s.ID)
	return err
}
func (b *managerTestBackend) Prompt(ctx context.Context, s Session, p Prompt) error {
	b.prompts.Add(1)
	if b.prompt != nil {
		return b.prompt(ctx, s, p)
	}
	return nil
}
func (b *managerTestBackend) SetMode(ctx context.Context, s Session, mode string) error {
	b.modes.Add(1)
	if b.setMode != nil {
		return b.setMode(ctx, s, mode)
	}
	return b.host.Publish(s.ID, []Event{{Type: "mode", Data: map[string]any{"mode": mode}}}, nil)
}
func (b *managerTestBackend) Cancel(context.Context, Session, string) error {
	b.cancels.Add(1)
	return nil
}
func (b *managerTestBackend) Attach(ctx context.Context, s Session) (Command, error) {
	b.attaches.Add(1)
	if b.attach != nil {
		return b.attach(ctx, s)
	}
	return Command{Path: "claude", Args: []string{"attach", s.NativeID}, Dir: s.Cwd}, nil
}
func (b *managerTestBackend) Close() error { b.closes.Add(1); return nil }
func (b *managerTestBackend) Shutdown(ctx context.Context) error {
	b.shutdowns.Add(1)
	if b.shutdown != nil {
		return b.shutdown(ctx)
	}
	return nil
}

func newTestManager(t *testing.T, dir string, b *managerTestBackend) *Manager {
	t.Helper()
	m, err := New(Options{StateDir: dir, Clock: quartz.NewMock(t), Factories: map[string]func(BackendOptions) (Backend, error){
		"claude": func(o BackendOptions) (Backend, error) { b.host = o.Host; return b, nil },
	}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func testManagerSession(t *testing.T, m *Manager) Session {
	t.Helper()
	s, err := m.Create(t.Context(), "claude", t.TempDir())
	require.NoError(t, err)
	return s
}

func TestManagerCreationFailure(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{
			name: "SetupFailedBeforeDispatch",
			err:  &creationSetupError{err: xerrors.New("native home is not a private directory")},
			want: "native claude setup failed before session creation: native home is not a private directory",
		},
		{
			name: "OutcomeUnknown",
			err:  xerrors.New("private native diagnostic"),
			want: "native creation outcome is unknown; inspect the native terminal before retrying",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			b := &managerTestBackend{create: func(context.Context, string) (Session, error) {
				return Session{}, test.err
			}}
			m := newTestManager(t, filepath.Join(t.TempDir(), "state"), b)
			_, err := m.Create(t.Context(), "claude", t.TempDir())
			require.EqualError(t, err, test.want)
			require.NotContains(t, err.Error(), "private native diagnostic")
			require.Empty(t, m.List())
			require.EqualValues(t, 1, b.creates.Load(), "failed creation must not be retried")
			require.Zero(t, b.resumes.Load())
		})
	}
}

func TestManagerLazyCreateAndPassiveClose(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	b := &managerTestBackend{}
	b.resume = func(_ context.Context, s Session) error {
		saved, err := b.host.Session(s.ID)
		if err != nil {
			return err
		}
		if !saved.ResumePending || saved.Mode != "human" {
			return xerrors.New("identity was not registered before readiness")
		}
		data, err := os.ReadFile(filepath.Join(dir, "sessions.json"))
		if err != nil {
			return err
		}
		var disk snapshot
		if err := json.Unmarshal(data, &disk); err != nil {
			return err
		}
		if disk.Sessions[s.ID] == nil {
			return xerrors.New("identity was not durable before resume")
		}
		return b.host.Publish(s.ID, []Event{{Type: "session.start", Data: map[string]any{"mode": "automation"}}}, json.RawMessage(`{"receipt":1}`))
	}
	m := newTestManager(t, dir, b)
	require.Zero(t, b.creates.Load())
	require.Zero(t, b.resumes.Load())
	s := testManagerSession(t, m)
	require.Equal(t, "human", s.Mode)
	require.False(t, s.ResumePending)
	require.JSONEq(t, `{"receipt":1}`, string(s.State))
	require.NoError(t, m.SetMode(t.Context(), s.ID, "automation"))
	command, err := m.Attach(t.Context(), s.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"attach", s.NativeID}, command.Args)
	require.EqualValues(t, 1, b.resumes.Load())
	require.EqualValues(t, 1, b.attaches.Load())
	require.NoError(t, m.Close())
	require.NoError(t, m.Close())
	require.EqualValues(t, 1, b.closes.Load())
	require.Zero(t, b.shutdowns.Load())
}

func TestManagerConcurrentDispatchAndCallerDisconnect(t *testing.T) {
	t.Parallel()
	entered := make(chan context.Context, 1)
	release := make(chan struct{})
	b := &managerTestBackend{prompt: func(ctx context.Context, _ Session, _ Prompt) error { entered <- ctx; <-release; return nil }}
	m := newTestManager(t, filepath.Join(t.TempDir(), "state"), b)
	s := testManagerSession(t, m)
	require.NoError(t, m.SetMode(t.Context(), s.ID, "automation"))
	p := Prompt{ID: uuid.NewString(), Text: "literal input"}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { result <- m.Submit(ctx, s.ID, p) }()
	nativeCtx := <-entered
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	require.NoError(t, nativeCtx.Err(), "caller disconnect cannot cancel native work")
	var wg sync.WaitGroup
	failures := make(chan error, 24)
	for range 24 {
		wg.Go(func() { failures <- m.Submit(t.Context(), s.ID, p) })
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	require.Error(t, m.Submit(t.Context(), s.ID, Prompt{ID: uuid.NewString(), Text: "another request"}))
	require.Error(t, m.Submit(t.Context(), s.ID, Prompt{ID: p.ID, Text: "different text"}))
	require.EqualValues(t, 1, b.prompts.Load())
	require.NoError(t, m.Cancel(t.Context(), s.ID, p.ID))
	require.NoError(t, m.Cancel(t.Context(), s.ID, p.ID))
	require.EqualValues(t, 1, b.cancels.Load())
	close(release)
	require.NoError(t, m.Close())
	require.ErrorIs(t, nativeCtx.Err(), context.Canceled)
}

func TestManagerAtomicObservationsAndSnapshots(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, filepath.Join(t.TempDir(), "state"), &managerTestBackend{})
	s := testManagerSession(t, m)
	data := map[string]any{"nested": map[string]any{"text": "original"}}
	state := json.RawMessage(`{"receipt":2}`)
	require.NoError(t, m.Publish(s.ID, []Event{{Type: "text", Data: data}}, state))
	data["nested"].(map[string]any)["text"] = "changed"
	state[0] = 'x'
	events, cursor, err := m.Events(t.Context(), s.ID, s.Cursor, false)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "original", events[0].Data["nested"].(map[string]any)["text"])
	events[0].Data["nested"].(map[string]any)["text"] = "reader mutation"
	current, err := m.Session(s.ID)
	require.NoError(t, err)
	require.JSONEq(t, `{"receipt":2}`, string(current.State))
	current.State[0] = 'x'
	listed := m.List()
	listed[0].State[0] = 'x'
	require.Error(t, m.Publish(s.ID, []Event{{Type: "text", Data: map[string]any{"text": strings.Repeat("x", maxEventBytes)}}}, json.RawMessage(`{"receipt":3}`)))
	current, err = m.Session(s.ID)
	require.NoError(t, err)
	require.Equal(t, cursor, current.Cursor)
	require.JSONEq(t, `{"receipt":2}`, string(current.State))
	require.NoError(t, m.Close())
	b := &managerTestBackend{}
	reopened := newTestManager(t, m.store.dir.file.Name(), b)
	events, _, err = reopened.Events(t.Context(), s.ID, s.Cursor, false)
	require.NoError(t, err)
	require.Equal(t, "original", events[0].Data["nested"].(map[string]any)["text"])
	require.Zero(t, b.resumes.Load())
}

func TestManagerRequestTombstonesSurviveRestart(t *testing.T) {
	t.Parallel()
	for _, completed := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown", true: "completed"}[completed], func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "state")
			m := newTestManager(t, dir, &managerTestBackend{})
			s := testManagerSession(t, m)
			require.NoError(t, m.SetMode(t.Context(), s.ID, "automation"))
			p := Prompt{ID: uuid.NewString(), Text: "do not replay me"}
			require.NoError(t, m.Submit(t.Context(), s.ID, p))
			if completed {
				require.NoError(t, m.Publish(s.ID, []Event{
					{Type: "turn.start", Data: map[string]any{"requestId": p.ID, "turnId": "turn"}},
					{Type: "turn.complete", Data: map[string]any{"requestId": p.ID, "turnId": "turn", "reason": "end_turn"}},
				}, nil))
			}
			require.NoError(t, m.Close())
			b := &managerTestBackend{}
			reopened := newTestManager(t, dir, b)
			require.NoError(t, reopened.Restore(t.Context()))
			require.NoError(t, reopened.Restore(t.Context()))
			require.EqualValues(t, 1, b.resumes.Load())
			require.Zero(t, b.creates.Load())
			require.Zero(t, b.prompts.Load())
			err := reopened.Submit(t.Context(), s.ID, p)
			if completed {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "unknown")
			}
			require.Error(t, reopened.Submit(t.Context(), s.ID, Prompt{ID: p.ID, Text: "changed"}))
			require.Zero(t, b.prompts.Load())
			current, err := reopened.Session(s.ID)
			require.NoError(t, err)
			if completed {
				require.Equal(t, "automation", current.Mode)
			} else {
				require.Equal(t, "human", current.Mode)
				require.NotEmpty(t, current.Fault)
			}
		})
	}
}

func TestManagerRestoreFailuresAreIndependentAndClaimed(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	m := newTestManager(t, dir, &managerTestBackend{})
	failed := testManagerSession(t, m)
	healthy := testManagerSession(t, m)
	require.NoError(t, m.Shutdown(t.Context()))
	require.NoError(t, m.Close())
	b := &managerTestBackend{}
	b.resume = func(_ context.Context, s Session) error {
		data, err := os.ReadFile(filepath.Join(dir, "sessions.json"))
		if err != nil {
			return err
		}
		var saved snapshot
		if err := json.Unmarshal(data, &saved); err != nil {
			return err
		}
		if !validUUID(saved.Sessions[s.ID].RestoreAttempt) || !saved.Sessions[s.ID].Session.ResumePending {
			return xerrors.New("missing durable restore claim")
		}
		if s.ID == failed.ID {
			return xerrors.New("private native diagnostic")
		}
		return nil
	}
	reopened := newTestManager(t, dir, b)
	require.Error(t, reopened.Restore(t.Context()))
	require.NoError(t, reopened.Restore(t.Context()))
	require.EqualValues(t, 2, b.resumes.Load())
	s, err := reopened.Session(failed.ID)
	require.NoError(t, err)
	require.True(t, s.Archived)
	require.NotContains(t, s.Fault, "private native diagnostic")
	s, err = reopened.Session(healthy.ID)
	require.NoError(t, err)
	require.False(t, s.Archived)
	require.Equal(t, "automation", s.Mode)
}

func TestManagerPersistenceFailurePreventsNativeDispatch(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	b := &managerTestBackend{}
	m := newTestManager(t, dir, b)
	s := testManagerSession(t, m)
	require.NoError(t, m.SetMode(t.Context(), s.ID, "automation"))
	require.NoError(t, os.Chmod(filepath.Join(dir, "sessions.json"), 0o644))
	require.ErrorContains(t, m.Submit(t.Context(), s.ID, Prompt{ID: uuid.NewString(), Text: "must not submit"}), "persistence")
	require.Zero(t, b.prompts.Load())
	require.NoError(t, os.Chmod(filepath.Join(dir, "sessions.json"), 0o600))
	require.Error(t, m.Submit(t.Context(), s.ID, Prompt{ID: uuid.NewString(), Text: "still blocked"}))
	require.Zero(t, b.prompts.Load())
	require.Error(t, m.Close())
}

func TestManagerJournalBoundsAndWait(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, filepath.Join(t.TempDir(), "state"), &managerTestBackend{})
	s := testManagerSession(t, m)
	batch := make([]Event, maxEvents)
	for i := range batch {
		batch[i] = Event{Type: "text", Data: map[string]any{"text": "bounded"}}
	}
	require.NoError(t, m.Publish(s.ID, batch, nil))
	current, err := m.Session(s.ID)
	require.NoError(t, err)
	require.Positive(t, current.HistoryStartCursor)
	_, _, err = m.Events(t.Context(), s.ID, 0, false)
	require.ErrorContains(t, err, "expired")
	events, _, err := m.Events(t.Context(), s.ID, current.HistoryStartCursor, false)
	require.NoError(t, err)
	require.Len(t, events, maxEvents)
	result := make(chan error, 1)
	go func() {
		events, _, err := m.Events(t.Context(), s.ID, current.Cursor, true)
		if err == nil && len(events) != 1 {
			err = xerrors.New("missing wake event")
		}
		result <- err
	}()
	require.NoError(t, m.Publish(s.ID, []Event{{Type: "text", Data: map[string]any{"text": "wake"}}}, nil))
	require.NoError(t, <-result)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = m.Events(ctx, s.ID, current.Cursor+1, true)
	require.ErrorIs(t, err, context.Canceled)
}

func TestManagerShutdownIsExplicitAndRetainsHistory(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	b := &managerTestBackend{}
	m := newTestManager(t, dir, b)
	spec := JobSpec{Harness: "claude", Cwd: t.TempDir(), Prompt: "only dispatch once"}
	job, err := m.RunJob(t.Context(), spec)
	require.NoError(t, err)
	s, err := m.Session(job.SessionID)
	require.NoError(t, err)
	require.NoError(t, m.Shutdown(t.Context()))
	require.NoError(t, m.Shutdown(t.Context()))
	require.EqualValues(t, 1, b.shutdowns.Load())
	_, err = m.Create(t.Context(), "claude", t.TempDir())
	require.Error(t, err)
	_, err = m.Session(s.ID)
	require.NoError(t, err)
	require.NoError(t, m.Close())
	resumed := make(chan Session, 1)
	reopenedBackend := &managerTestBackend{resume: func(_ context.Context, saved Session) error { resumed <- saved; return nil }}
	reopened := newTestManager(t, dir, reopenedBackend)
	require.NoError(t, reopened.Restore(t.Context()))
	require.NoError(t, reopened.Restore(t.Context()))
	require.EqualValues(t, 1, reopenedBackend.resumes.Load())
	restored := <-resumed
	require.Equal(t, s.ID, restored.ID)
	require.Equal(t, s.NativeID, restored.NativeID)
	require.Equal(t, s.Cwd, restored.Cwd)
	require.Len(t, reopened.List(), 1)
	restoredJob, err := reopened.RunJob(t.Context(), spec)
	require.NoError(t, err)
	require.Equal(t, job.RequestID, restoredJob.RequestID)
	require.Equal(t, "unknown", restoredJob.Status)
	require.Error(t, reopened.Submit(t.Context(), s.ID, Prompt{ID: job.RequestID, Text: spec.Prompt}))
	require.Zero(t, reopenedBackend.creates.Load())
	require.Zero(t, reopenedBackend.prompts.Load())
}
