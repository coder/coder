package agentacp

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func requireClosed[T any](ctx context.Context, t *testing.T, events <-chan T) {
	t.Helper()
	select {
	case event, ok := <-events:
		require.False(t, ok, "unexpected event: %#v", event)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestSessionWatchReplayAndIsolation(t *testing.T) {
	t.Parallel()
	s, _, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusRunning)
	ctx := testutil.Context(t, testutil.WaitLong)
	s.mu.Lock()
	s.info.Harness.ConfigOptions = []workspacesdk.ACPConfigOption{{ID: "original"}}
	s.mu.Unlock()
	before := sessionNotification(s.id, "before")
	require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &before}))
	read, err := s.Read(ctx, nil)
	require.NoError(t, err)
	for _, cursor := range []workspacesdk.ACPCursor{
		{Epoch: uuid.New()},
		{Epoch: read.Session.Cursor.Epoch, Seq: read.Session.Cursor.Seq + 1},
	} {
		_, events, err := s.Watch(ctx, &cursor)
		require.ErrorIs(t, err, ErrConflict)
		require.Nil(t, events)
	}
	baseline, first, err := s.Watch(ctx, &read.Session.Cursor)
	require.NoError(t, err)
	require.Empty(t, baseline.Events)
	require.Empty(t, baseline.AssistantResponse)
	baseline, second, err := s.Watch(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, read, baseline)
	baseline.Events[0].Update[0] = 'x'
	baseline.Session.Harness.ConfigOptions[0].ID = "mutated"
	snapshot, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, read, snapshot)
	after := sessionNotification(s.id, "after")
	require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &after}))
	one := testutil.RequireReceive(ctx, t, first)
	two := testutil.RequireReceive(ctx, t, second)
	require.Equal(t, one, two)
	require.Equal(t, read.Session.Cursor.Seq+1, one.Cursor.Seq)
	one.Update[0] = 'x'
	current, err := s.Read(ctx, &read.Session.Cursor)
	require.NoError(t, err)
	require.Equal(t, []workspacesdk.ACPEvent{two}, current.Events)
	require.Equal(t, "after", current.AssistantResponse)

	require.NoError(t, s.adapterError(ctx, xerrors.New("test failure")))
	for _, events := range []<-chan workspacesdk.ACPEvent{first, second} {
		require.Equal(t, workspacesdk.ACPEventKindError, testutil.RequireReceive(ctx, t, events).Kind)
	}
	one = testutil.RequireReceive(ctx, t, first)
	two = testutil.RequireReceive(ctx, t, second)
	require.Equal(t, workspacesdk.ACPSessionStatusError, two.Session.Status)
	one.Session.Status = workspacesdk.ACPSessionStatusIdle
	require.Equal(t, workspacesdk.ACPSessionStatusError, two.Session.Status)
	require.ErrorIs(t, s.adapterError(ctx, xerrors.New("another failure")), ErrConflict)
	current, err = s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, *two.Session, current.Session)
	waited, err := s.Wait(ctx)
	require.ErrorContains(t, err, "test failure")
	require.Equal(t, current, waited)
	s.shutdown()
	requireClosed(ctx, t, first)
	requireClosed(ctx, t, second)
}

func TestSessionWatchOverflowAndCleanup(t *testing.T) {
	t.Parallel()
	s, _, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusRunning)
	ctx := testutil.Context(t, testutil.WaitLong)
	slowCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	_, slow, err := s.Watch(slowCtx, nil)
	require.NoError(t, err)
	_, fast, err := s.Watch(ctx, nil)
	require.NoError(t, err)
	for i := range uint64(65) {
		notification := sessionNotification(s.id, fmt.Sprint(i))
		require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &notification}))
		event := testutil.RequireReceive(ctx, t, fast)
		require.Equal(t, i+1, event.Cursor.Seq)
	}
	for i := range uint64(64) {
		event := testutil.RequireReceive(ctx, t, slow)
		require.Equal(t, i+1, event.Cursor.Seq)
	}
	requireClosed(ctx, t, slow)
	cancel()
	cancel()
	read, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Len(t, read.Events, 65)

	// An evicted subscriber does not affect subsequent publications.
	notification := sessionNotification(s.id, "still running")
	require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &notification}))
	require.Equal(t, uint64(66), testutil.RequireReceive(ctx, t, fast).Cursor.Seq)
	s.shutdown()
	requireClosed(ctx, t, fast)
	s.mu.Lock()
	defer s.mu.Unlock()
	require.Empty(t, s.subscribers)
}

func TestSessionWatchCancellation(t *testing.T) {
	t.Parallel()
	s, _, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusRunning)
	ctx := testutil.Context(t, testutil.WaitLong)
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	_, events, err := s.Watch(watchCtx, nil)
	require.NoError(t, err)
	cancel()
	cancel()
	requireClosed(ctx, t, events)
	_, events, err = s.Watch(watchCtx, nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, events)

	notification := sessionNotification(s.id, "not interrupted")
	require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &notification}))
	read, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusRunning, read.Session.Status)
	require.Equal(t, "not interrupted", read.AssistantResponse)
	s.shutdown()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.Empty(t, s.subscribers)
}

func TestWaitSessionPartialOutput(t *testing.T) {
	t.Parallel()
	for _, ending := range []string{"failure", "cancellation", "shutdown", "overflow"} {
		t.Run(ending, func(t *testing.T) {
			t.Parallel()
			s, writes, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusIdle)
			ctx := testutil.Context(t, testutil.WaitLong)
			admission, err := s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hello"})
			require.NoError(t, err)
			require.Equal(t, workspacesdk.ACPMessageOutcomePrompt, admission.Outcome)
			testutil.RequireReceive(ctx, t, writes)
			notification := sessionNotification(s.id, "partial output")
			require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &notification}))
			watchCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			baseline, events, err := s.Watch(watchCtx, nil)
			require.NoError(t, err)
			switch ending {
			case "failure":
				_, err := s.Interrupt(ctx)
				require.NoError(t, err)
				require.NoError(t, s.adapterError(ctx, xerrors.New("test failure")))
			case "cancellation":
				cancel()
			case "shutdown":
				s.shutdown()
			case "overflow":
				for range 65 {
					notification := sessionNotification(s.id, "")
					require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &notification}))
				}
			}
			result, err := waitSession(watchCtx, baseline, events)
			switch ending {
			case "failure":
				require.ErrorContains(t, err, "test failure")
				require.Equal(t, workspacesdk.ACPSessionStatusError, result.Session.Status)
			case "cancellation":
				require.ErrorIs(t, err, context.Canceled)
			default:
				require.ErrorIs(t, err, ErrUnavailable)
			}
			require.Equal(t, "partial output", result.AssistantResponse)
			require.NotEmpty(t, result.Events)
		})
	}
}

func TestSessionWatchConcurrentPublication(t *testing.T) {
	t.Parallel()
	s, _, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusRunning)
	ctx := testutil.Context(t, testutil.WaitLong)
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type watchResult struct {
		read   workspacesdk.ACPSessionResponse
		events <-chan workspacesdk.ACPEvent
		err    error
	}
	replies := make(chan watchResult, 1)
	go func() {
		baseline, events, err := s.Watch(watchCtx, nil)
		replies <- watchResult{read: baseline, events: events, err: err}
	}()
	notification := sessionNotification(s.id, "concurrent output")
	require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &notification}))
	watch := testutil.RequireReceive(ctx, t, replies)
	require.NoError(t, watch.err)
	read, err := s.Read(ctx, nil)
	require.NoError(t, err)
	received := watch.read.Events
	if len(received) == 0 {
		received = append(received, testutil.RequireReceive(ctx, t, watch.events))
	}
	require.Equal(t, read.Events, received)
	cancel()
	requireClosed(ctx, t, watch.events)
	s.shutdown()
}

func TestWaitSessionPreservesCompletedTurn(t *testing.T) {
	t.Parallel()
	s, _, _ := newRuntimeSession(t, workspacesdk.ACPSessionStatusRunning)
	ctx := testutil.Context(t, testutil.WaitLong)
	// Historical idle events must not complete a wait.
	s.mu.Lock()
	s.info.Status = workspacesdk.ACPSessionStatusIdle
	s.appendEventLocked(workspacesdk.ACPEventKindStatus, "", nil)
	s.info.Status = workspacesdk.ACPSessionStatusRunning
	s.appendEventLocked(workspacesdk.ACPEventKindStatus, "", nil)
	s.mu.Unlock()
	baseline, watched, err := s.Watch(ctx, nil)
	require.NoError(t, err)
	for _, text := range []string{"final ", "output"} {
		notification := sessionNotification(s.id, text)
		require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &notification}))
	}
	require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Prompt: &acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, Turn: 1}))
	completed, err := s.Read(ctx, nil)
	require.NoError(t, err)
	_, err = s.Message(ctx, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "next turn"})
	require.NoError(t, err)
	current, err := s.Read(ctx, nil)
	require.NoError(t, err)
	require.NotEqual(t, completed.Session.Cursor.Epoch, current.Session.Cursor.Epoch)

	// Drain only after the FSM has discarded the old transcript.
	waited, err := waitSession(ctx, baseline, watched)
	require.NoError(t, err)
	require.Equal(t, completed, waited)
	require.Equal(t, "final output", waited.AssistantResponse)
	for _, expected := range current.Events {
		require.Equal(t, expected, testutil.RequireReceive(ctx, t, watched))
	}
}
