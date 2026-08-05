package chatd

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

type fakeAgentGate struct {
	mu       sync.Mutex
	chats    map[uuid.UUID]*fakeGateChat
	yieldErr error
}

type fakeGateChat struct {
	admitCh chan struct{}

	mu       sync.Mutex
	admitted bool
	events   []string
}

func newFakeAgentGate() *fakeAgentGate {
	return &fakeAgentGate{chats: make(map[uuid.UUID]*fakeGateChat)}
}

func (f *fakeAgentGate) chat(chatID uuid.UUID) *fakeGateChat {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.chats[chatID]; ok {
		return c
	}
	c := &fakeGateChat{admitCh: make(chan struct{})}
	f.chats[chatID] = c
	return c
}

func (f *fakeAgentGate) admit(chatID uuid.UUID) {
	c := f.chat(chatID)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.admitted {
		return
	}
	c.admitted = true
	close(c.admitCh)
}

func (f *fakeAgentGate) Acquire(ctx context.Context, chatID uuid.UUID, _ uuid.UUID) error {
	c := f.chat(chatID)
	c.record("acquire")
	select {
	case <-c.admitCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *fakeAgentGate) Yield(_ context.Context, chatID uuid.UUID, _ uuid.UUID) error {
	f.chat(chatID).record("yield")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.yieldErr
}

func (c *fakeGateChat) record(event string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *fakeGateChat) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.events)
}

func (c *fakeGateChat) waitForEvent(t *testing.T, event string) {
	t.Helper()
	require.Eventually(t, func() bool {
		return slices.Contains(c.snapshot(), event)
	}, testutil.WaitLong, testutil.IntervalFast, "expected gate event %q, got %v", event, c.snapshot())
}

func countEvents(events []string, event string) int {
	count := 0
	for _, e := range events {
		if e == event {
			count++
		}
	}
	return count
}

func testFakeGateOptions(t *testing.T, f *workerTestFixture, starter chatWorkerTaskStarter) (chatWorkerOptions, *fakeAgentGate) {
	t.Helper()
	opts := testOptions(t, f, starter)
	gate := newFakeAgentGate()
	opts.AgentGate = gate
	return opts, gate
}

func TestWorker_AgentGateGatesGeneration(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newBlockingTaskStarter(false)
	opts, gate := testFakeGateOptions(t, f, starter)

	chatA := f.createRunningChat(t)
	chatB := f.createRunningChat(t)
	startWorker(t, opts)

	gate.chat(chatA.ID).waitForEvent(t, "acquire")
	gate.chat(chatB.ID).waitForEvent(t, "acquire")
	starter.assertNoCall(t)

	gate.admit(chatA.ID)
	call := starter.waitCall(t, taskKindGeneration, uuid.Nil)
	require.Equal(t, chatA.ID, call.input.ChatID)
	starter.assertNoCall(t)

	gate.admit(chatB.ID)
	call = starter.waitCall(t, taskKindGeneration, uuid.Nil)
	require.Equal(t, chatB.ID, call.input.ChatID)
}

func TestWorker_AgentGateInterruptDoesNotWaitForSlot(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newBlockingTaskStarter(false)
	opts, gate := testFakeGateOptions(t, f, starter)

	chat := f.createRunningChat(t)
	startWorker(t, opts)
	gate.chat(chat.ID).waitForEvent(t, "acquire")

	interruptChat(t, f, chat.ID)
	call := starter.waitCall(t, taskKindInterrupt, chat.ID)
	require.Equal(t, chat.ID, call.input.ChatID)
}

type waitAgentStarter struct {
	*recordingTaskStarter
	leaseOK   chan bool
	resumeErr chan error
}

func newWaitAgentStarter() *waitAgentStarter {
	return &waitAgentStarter{
		recordingTaskStarter: newRecordingTaskStarter(),
		leaseOK:              make(chan bool, 1),
		resumeErr:            make(chan error, 1),
	}
}

func (s *waitAgentStarter) StartGeneration(ctx context.Context, input chatWorkerTaskStartInput) error {
	lease, ok := agentSlotLeaseFromContext(ctx)
	s.leaseOK <- ok
	if !ok {
		return errors.Join(errTaskExpectedExit, xerrors.New("no agent slot lease in generation context"))
	}
	if err := lease.Pause(ctx); err != nil {
		return err
	}
	s.resumeErr <- lease.Resume(ctx)
	return s.recordingTaskStarter.StartGeneration(ctx, input)
}

func TestWorker_AgentGateWaitAgentPauseResume(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newWaitAgentStarter()
	opts, gate := testFakeGateOptions(t, f, starter)

	chat := f.createRunningChat(t)
	gate.admit(chat.ID)
	startWorker(t, opts)

	ctx := testutil.Context(t, testutil.WaitLong)
	require.True(t, testutil.RequireReceive(ctx, t, starter.leaseOK))
	require.NoError(t, testutil.RequireReceive(ctx, t, starter.resumeErr))

	events := gate.chat(chat.ID).snapshot()
	require.Equal(t, 1, countEvents(events, "yield"))
	require.GreaterOrEqual(t, countEvents(events, "acquire"), 2)
}

func TestAgentSlotLease_PauseYieldFailure(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	gate := newFakeAgentGate()
	chatID := uuid.New()
	gate.admit(chatID)
	lease := newAgentSlotLease(gate, chatID, uuid.Nil)
	require.NoError(t, lease.EnsureHeld(ctx))

	gate.mu.Lock()
	gate.yieldErr = xerrors.New("yield unavailable")
	gate.mu.Unlock()
	require.ErrorContains(t, lease.Pause(ctx), "yield unavailable")

	// The failed pause restored the refcount, so the next pause yields again.
	gate.mu.Lock()
	gate.yieldErr = nil
	gate.mu.Unlock()
	require.NoError(t, lease.Pause(ctx))
	require.Equal(t, 2, countEvents(gate.chat(chatID).snapshot(), "yield"))
	require.NoError(t, lease.Resume(ctx))
}

func TestWorker_NoGateLeavesGenerationUncapped(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)

	chat := f.createRunningChat(t)
	startWorker(t, opts)

	call := starter.waitCall(t, taskKindGeneration, chat.ID)
	_, ok := agentSlotLeaseFromContext(call.ctx)
	require.False(t, ok)
}

func TestAgentSlotLease_PauseRefCounting(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	gate := newFakeAgentGate()
	chatID := uuid.New()
	gate.admit(chatID)
	lease := newAgentSlotLease(gate, chatID, uuid.Nil)

	require.NoError(t, lease.Pause(ctx))
	require.NoError(t, lease.Pause(ctx))
	require.NoError(t, lease.Resume(ctx))
	require.NoError(t, lease.Resume(ctx))

	events := gate.chat(chatID).snapshot()
	require.Equal(t, 1, countEvents(events, "yield"))
	require.Equal(t, 1, countEvents(events, "acquire"))
}

func TestChatMachine_CapacityNudgeOnRelease(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	chat := f.createRunningChat(t)
	_, err := f.db.SetChatConcurrencyState(ctx, database.SetChatConcurrencyStateParams{
		ID: chat.ID,
		ConcurrencyState: database.NullChatConcurrencyState{
			ChatConcurrencyState: database.ChatConcurrencyStateActive,
			Valid:                true,
		},
	})
	require.NoError(t, err)

	nudges := make(chan struct{}, 8)
	unsubscribe, err := f.pubsub.Subscribe(coderdpubsub.ChatCapacityChannel, func(_ context.Context, _ []byte) {
		nudges <- struct{}{}
	})
	require.NoError(t, err)
	defer unsubscribe()

	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

	// running -> interrupting keeps the claim: no nudge.
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.Interrupt(chatstate.InterruptInput{})
		return err
	}))
	select {
	case <-nudges:
		t.Fatal("capacity nudge published for a transition that kept the claim")
	case <-time.After(testutil.IntervalMedium):
	}

	// interrupting -> waiting clears the claim: one nudge.
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishInterruption(chatstate.FinishInterruptionInput{})
		return err
	}))
	testutil.RequireReceive(ctx, t, nudges)
}
