//go:build linux

package agentnative

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestManagerInitialJobOnceOnly(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	entered := make(chan struct{})
	release := make(chan struct{})
	b := &managerTestBackend{}
	b.create = func(_ context.Context, cwd string) (Session, error) {
		data, err := os.ReadFile(filepath.Join(dir, "sessions.json"))
		if err != nil {
			return Session{}, err
		}
		var saved snapshot
		if err := json.Unmarshal(data, &saved); err != nil {
			return Session{}, err
		}
		if saved.Job == nil || saved.Job.Status != "starting" || !validUUID(saved.Job.RequestID) {
			return Session{}, xerrors.New("job creation was not durably claimed")
		}
		close(entered)
		<-release
		id := uuid.NewString()
		return Session{ID: id, NativeID: id, Harness: "claude", Cwd: cwd, Mode: "human"}, nil
	}
	b.prompt = func(_ context.Context, s Session, p Prompt) error {
		return b.host.Publish(s.ID, []Event{
			{Type: "turn.start", Data: map[string]any{"requestId": p.ID, "turnId": "native-turn"}},
			{Type: "turn.complete", Data: map[string]any{"requestId": p.ID, "turnId": "native-turn", "reason": "end_turn"}},
		}, json.RawMessage(`{"completedReceipt":true}`))
	}
	m := newTestManager(t, dir, b)
	spec := JobSpec{Harness: "claude", Cwd: t.TempDir(), Prompt: "initial task"}
	result := make(chan operationResult[Job], 1)
	go func() { job, err := m.RunJob(t.Context(), spec); result <- operationResult[Job]{job, err} }()
	<-entered
	pending, err := m.RunJob(t.Context(), spec)
	require.NoError(t, err)
	require.Equal(t, "starting", pending.Status)
	require.Empty(t, pending.SessionID)
	close(release)
	completed := <-result
	require.NoError(t, completed.err)
	require.Equal(t, "completed", completed.value.Status)
	require.Equal(t, pending.RequestID, completed.value.RequestID)
	require.EqualValues(t, 1, b.creates.Load())
	require.EqualValues(t, 1, b.prompts.Load())
	require.NoError(t, m.Close())
	newBackend := &managerTestBackend{}
	reopened := newTestManager(t, dir, newBackend)
	restored, err := reopened.RunJob(t.Context(), spec)
	require.NoError(t, err)
	require.Equal(t, completed.value, restored)
	spec.Prompt = "different initial task"
	_, err = reopened.RunJob(t.Context(), spec)
	require.ErrorContains(t, err, "different input")
	require.Zero(t, newBackend.creates.Load())
	require.Zero(t, newBackend.prompts.Load())
}

func TestManagerFailedJobCreationNeverRetried(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	b := &managerTestBackend{create: func(context.Context, string) (Session, error) { return Session{}, xerrors.New("may have created") }}
	m := newTestManager(t, dir, b)
	spec := JobSpec{Cwd: t.TempDir(), Prompt: "initial task"}
	job, err := m.RunJob(t.Context(), spec)
	require.Error(t, err)
	require.Equal(t, "unknown", job.Status)
	require.Empty(t, job.SessionID)
	require.NoError(t, m.Close())
	newBackend := &managerTestBackend{}
	reopened := newTestManager(t, dir, newBackend)
	job, err = reopened.RunJob(t.Context(), spec)
	require.NoError(t, err)
	require.Equal(t, "unknown", job.Status)
	require.Zero(t, newBackend.creates.Load())
	require.Zero(t, newBackend.prompts.Load())
}

func TestManagerJobRetainsIdentityOnReadinessFailure(t *testing.T) {
	t.Parallel()
	b := &managerTestBackend{resume: func(context.Context, Session) error { return xerrors.New("readiness failed") }}
	m := newTestManager(t, filepath.Join(t.TempDir(), "state"), b)
	job, err := m.RunJob(t.Context(), JobSpec{Cwd: t.TempDir(), Prompt: "initial task"})
	require.Error(t, err)
	require.NotEmpty(t, job.SessionID)
	require.Equal(t, "unknown", job.Status)
	_, err = m.Session(job.SessionID)
	require.NoError(t, err)
	require.Zero(t, b.prompts.Load())
}

func TestManagerAbortedTurnCancelsRequestAndJob(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	b := &managerTestBackend{}
	b.prompt = func(_ context.Context, s Session, p Prompt) error {
		return b.host.Publish(s.ID, []Event{
			{Type: "turn.start", Data: map[string]any{"requestId": p.ID, "turnId": "native-turn", "text": p.Text}},
			{Type: "turn.complete", Data: map[string]any{"requestId": p.ID, "turnId": "native-turn", "reason": "aborted", "answer": ""}},
		}, nil)
	}
	m := newTestManager(t, dir, b)
	spec := JobSpec{Cwd: t.TempDir(), Prompt: "initial task"}
	job, err := m.RunJob(t.Context(), spec)
	require.NoError(t, err)
	require.Equal(t, "canceled", job.Status)
	m.mu.Lock()
	request := m.state.Sessions[job.SessionID].Requests[job.RequestID]
	m.mu.Unlock()
	require.Equal(t, "canceled", request.Status)
	s, err := m.Session(job.SessionID)
	require.NoError(t, err)
	require.Empty(t, s.ActiveRequestID)
	require.Empty(t, s.ActiveTurnID)
	events, _, err := m.Events(t.Context(), s.ID, 0, false)
	require.NoError(t, err)
	require.Equal(t, "aborted", events[len(events)-1].Data["reason"], "retain the canonical reason for ACP")
	require.NoError(t, m.Close())
	newBackend := &managerTestBackend{}
	reopened := newTestManager(t, dir, newBackend)
	require.NoError(t, reopened.Restore(t.Context()))
	restored, err := reopened.RunJob(t.Context(), spec)
	require.NoError(t, err)
	require.Equal(t, job, restored)
	require.NoError(t, reopened.Submit(t.Context(), s.ID, Prompt{ID: job.RequestID, Text: spec.Prompt}))
	require.Zero(t, newBackend.creates.Load())
	require.Zero(t, newBackend.prompts.Load())
}
