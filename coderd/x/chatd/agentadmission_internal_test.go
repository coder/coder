package chatd

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type fakeAdmission struct {
	mu      sync.Mutex
	refused map[uuid.UUID]bool
}

func newFakeAdmission() *fakeAdmission {
	return &fakeAdmission{refused: make(map[uuid.UUID]bool)}
}

func (f *fakeAdmission) refuse(chatID uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refused[chatID] = true
}

func (f *fakeAdmission) allow(chatID uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.refused, chatID)
}

func (f *fakeAdmission) Admit(_ context.Context, _ database.Store, chat database.Chat) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.refused[chat.ID], nil
}

func capacityQueuedAt(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID) *database.Chat {
	t.Helper()
	chat, err := db.GetChatByID(ctx, chatID)
	require.NoError(t, err)
	return &chat
}

func chatOwnershipMessages(t *testing.T, ps *recordingPubsub, chatID uuid.UUID) int {
	t.Helper()
	count := 0
	for _, msg := range ps.ownershipMessages(t) {
		if msg.ChatID == chatID {
			count++
		}
	}
	return count
}

func capacityEvents(t *testing.T, ps *recordingPubsub, chatID uuid.UUID) []codersdk.ChatWatchEvent {
	t.Helper()
	events := make([]codersdk.ChatWatchEvent, 0)
	for _, event := range ps.watchEvents(t) {
		if event.Kind == codersdk.ChatWatchEventKindCapacityChange && event.Chat.ID == chatID {
			events = append(events, event)
		}
	}
	return events
}

func TestWorker_AdmissionRefusalQueuesChat(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	recording := newRecordingPubsub(f.pubsub)
	starter := newRecordingTaskStarter()
	admission := newFakeAdmission()
	opts := testOptions(t, f, starter)
	opts.Pubsub = recording
	opts.AgentAdmission = admission

	chat := f.createRunningChat(t)
	admission.refuse(chat.ID)
	startWorker(t, opts)

	ctx := testutil.Context(t, testutil.WaitLong)
	require.Eventually(t, func() bool {
		return capacityQueuedAt(ctx, t, f.db, chat.ID).CapacityQueuedAt.Valid
	}, testutil.WaitLong, testutil.IntervalFast)
	starter.assertNoCall(t)

	events := capacityEvents(t, recording, chat.ID)
	require.NotEmpty(t, events)
	require.NotNil(t, events[len(events)-1].Chat.QueuedForCapacityAt)

	// The recorder wraps only worker pubsub, so an ownership hint here would
	// prove a refusal can wake workers into an immediate retry loop.
	require.Equal(t, 0, chatOwnershipMessages(t, recording, chat.ID))
}

func TestWorker_AdmissionAdmitClearsQueueMark(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	recording := newRecordingPubsub(f.pubsub)
	starter := newRecordingTaskStarter()
	admission := newFakeAdmission()
	opts := testOptions(t, f, starter)
	opts.Pubsub = recording
	opts.AgentAdmission = admission

	chat := f.createRunningChat(t)
	admission.refuse(chat.ID)
	worker := startWorker(t, opts)

	ctx := testutil.Context(t, testutil.WaitLong)
	require.Eventually(t, func() bool {
		return capacityQueuedAt(ctx, t, f.db, chat.ID).CapacityQueuedAt.Valid
	}, testutil.WaitLong, testutil.IntervalFast)

	admission.allow(chat.ID)
	worker.Wake()

	call := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Equal(t, chat.ID, call.input.ChatID)
	require.False(t, capacityQueuedAt(ctx, t, f.db, chat.ID).CapacityQueuedAt.Valid)

	require.Eventually(t, func() bool {
		events := capacityEvents(t, recording, chat.ID)
		return len(events) >= 2 && events[len(events)-1].Chat.QueuedForCapacityAt == nil
	}, testutil.WaitLong, testutil.IntervalFast)
}

func TestWorker_InterruptingSortsBeforeCapacityQueue(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	queued := f.createRunningChat(t)
	marked, err := f.db.MarkChatCapacityQueued(ctx, database.MarkChatCapacityQueuedParams{
		ID:           queued.ID,
		StaleSeconds: 30,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, marked)
	interrupting := f.createRunningChat(t)
	interruptChat(t, f, interrupting.ID)
	requiresAction := f.createRequiresActionChat(t)

	rows, err := f.db.GetChatWorkerAcquisitionCandidates(ctx, database.GetChatWorkerAcquisitionCandidatesParams{
		StaleSeconds: 30,
		LimitCount:   10,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(rows), 3)
	require.Equal(t, interrupting.ID, rows[0].ID, "interrupting chats must sort before the capacity queue")
	require.Equal(t, requiresAction.ID, rows[1].ID, "requires_action chats bypass admission and must sort before the capacity queue")
	require.Equal(t, queued.ID, rows[2].ID)
}

// rootRefusingAdmission simulates a full root pool with free subagent
// capacity: running root chats refuse, everything else admits.
type rootRefusingAdmission struct {
	mu    sync.Mutex
	calls int
}

func (a *rootRefusingAdmission) Admit(_ context.Context, _ database.Store, chat database.Chat) (bool, error) {
	a.mu.Lock()
	a.calls++
	a.mu.Unlock()
	if chat.Status != database.ChatStatusRunning {
		return true, nil
	}
	return chat.ParentChatID.Valid, nil
}

func (a *rootRefusingAdmission) callCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

func TestWorker_FullPoolDoesNotStarveOtherPool(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AgentAdmission = &rootRefusingAdmission{}
	ctx := testutil.Context(t, testutil.WaitLong)

	// A pre-marked root backlog deeper than two batches; the subagent is
	// created last, so an ordering that ignores pools buries it behind a
	// batch of pure re-skips and the pass ends before reaching it.
	roots := make([]database.Chat, 0, 2*int(opts.AcquisitionBatchSize)+5)
	for range cap(roots) {
		chat := f.createRunningChat(t)
		marked, err := f.db.MarkChatCapacityQueued(ctx, database.MarkChatCapacityQueuedParams{
			ID:           chat.ID,
			StaleSeconds: 30,
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, marked)
		roots = append(roots, chat)
	}
	sub := f.createRunningSubagentChat(t, roots[0].ID)

	startWorker(t, opts)

	call := starter.waitCall(t, taskKindGeneration, sub.ID)
	require.Equal(t, sub.ID, call.input.ChatID)
}

// A configured batch size of 1 would only ever surface the
// tie-break-favored root pool: with two already-marked queued roots the
// pass refuses one, re-skips the other without progress, and ends
// before examining the subagent pool. The floor of 2 keeps both pool
// heads in every batch.
func TestWorker_BatchSizeOneCannotHideAPool(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AgentAdmission = &rootRefusingAdmission{}
	opts.AcquisitionBatchSize = 1
	ctx := testutil.Context(t, testutil.WaitLong)

	roots := []database.Chat{f.createRunningChat(t), f.createRunningChat(t)}
	for _, chat := range roots {
		marked, err := f.db.MarkChatCapacityQueued(ctx, database.MarkChatCapacityQueuedParams{
			ID:           chat.ID,
			StaleSeconds: 30,
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, marked)
	}
	sub := f.createRunningSubagentChat(t, roots[0].ID)

	startWorker(t, opts)

	call := starter.waitCall(t, taskKindGeneration, sub.ID)
	require.Equal(t, sub.ID, call.input.ChatID)
}

// After the first refusal proves a pool full, the rest of the pass must
// queue-mark that pool's chats without opening refusal transactions.
// All-marked is the pass-completion signal, so the count is stable when
// read.
func TestWorker_FullPoolSkipsRefusalsAfterFirst(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	admission := &rootRefusingAdmission{}
	opts.AgentAdmission = admission
	ctx := testutil.Context(t, testutil.WaitLong)

	chats := make([]database.Chat, 5)
	for i := range chats {
		chats[i] = f.createRunningChat(t)
	}
	startWorker(t, opts)

	require.Eventually(t, func() bool {
		for _, chat := range chats {
			if !capacityQueuedAt(ctx, t, f.db, chat.ID).CapacityQueuedAt.Valid {
				return false
			}
		}
		return true
	}, testutil.WaitLong, testutil.IntervalFast)
	require.LessOrEqual(t, admission.callCount(), 2,
		"a full pool must be skipped after one refusal, not re-refused per chat")
}

func TestWorker_AdmissionAdmitsInQueueOrder(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AgentAdmission = newFakeAdmission()
	ctx := testutil.Context(t, testutil.WaitLong)

	// Queue the newer chat first so FIFO order by capacity_queued_at
	// disagrees with the fallback updated_at order.
	older := f.createRunningChat(t)
	newer := f.createRunningChat(t)
	for _, id := range []uuid.UUID{newer.ID, older.ID} {
		marked, err := f.db.MarkChatCapacityQueued(ctx, database.MarkChatCapacityQueuedParams{
			ID:           id,
			StaleSeconds: 30,
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, marked)
	}
	startWorker(t, opts)

	first := starter.waitCall(t, taskKindGeneration, uuid.Nil)
	require.Equal(t, newer.ID, first.input.ChatID, "the longer-queued chat must admit first")
	second := starter.waitCall(t, taskKindGeneration, uuid.Nil)
	require.Equal(t, older.ID, second.input.ChatID)
}

type runningRefusingAdmission struct{}

func (runningRefusingAdmission) Admit(_ context.Context, _ database.Store, chat database.Chat) (bool, error) {
	return chat.Status != database.ChatStatusRunning, nil
}

func TestWorker_InterruptClaimsCapacityQueuedChat(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AgentAdmission = runningRefusingAdmission{}

	chat := f.createRunningChat(t)
	worker := startWorker(t, opts)

	ctx := testutil.Context(t, testutil.WaitLong)
	require.Eventually(t, func() bool {
		return capacityQueuedAt(ctx, t, f.db, chat.ID).CapacityQueuedAt.Valid
	}, testutil.WaitLong, testutil.IntervalFast)

	interruptChat(t, f, chat.ID)
	worker.Wake()

	call := starter.waitCall(t, taskKindInterrupt, chat.ID)
	require.Equal(t, chat.ID, call.input.ChatID)
}

// One acquisition pass must page past a full pool's backlog (via
// @exclude_ids) and queue-mark every skipped chat, not just the first
// batch.
func TestWorker_AdmissionPassReachesChatsBeyondRefusedBatch(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AgentAdmission = &rootRefusingAdmission{}
	opts.AcquisitionBatchSize = 2
	ctx := testutil.Context(t, testutil.WaitLong)

	chats := make([]database.Chat, 5)
	for i := range chats {
		chats[i] = f.createRunningChat(t)
	}
	startWorker(t, opts)

	require.Eventually(t, func() bool {
		for _, chat := range chats {
			if !capacityQueuedAt(ctx, t, f.db, chat.ID).CapacityQueuedAt.Valid {
				return false
			}
		}
		return true
	}, testutil.WaitLong, testutil.IntervalFast)
}

func TestWorker_RunnerPublishesCapacityReleaseNudge(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	recording := newRecordingPubsub(f.pubsub)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.Pubsub = recording
	opts.AgentAdmission = newFakeAdmission()

	chat := f.createRunningChat(t)
	startWorker(t, opts)
	starter.waitCall(t, taskKindGeneration, chat.ID)

	ctx := testutil.Context(t, testutil.WaitLong)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishError(chatstate.FinishErrorInput{
			LastError: pqtype.NullRawMessage{RawMessage: []byte(`{"message":"boom"}`), Valid: true},
		})
		return err
	}))

	// FinishError publishes no ownership hint, so any recorded hint is the
	// runner's capacity-release nudge.
	require.Eventually(t, func() bool {
		return chatOwnershipMessages(t, recording, chat.ID) >= 1
	}, testutil.WaitLong, testutil.IntervalFast)
}

func TestChat_StatusTransitionClearsCapacityQueueMark(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	chat := f.createRunningChat(t)
	marked, err := f.db.MarkChatCapacityQueued(ctx, database.MarkChatCapacityQueuedParams{
		ID:           chat.ID,
		StaleSeconds: 30,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, marked)

	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishError(chatstate.FinishErrorInput{
			LastError: pqtype.NullRawMessage{RawMessage: []byte(`{"message":"boom"}`), Valid: true},
		})
		return err
	}))

	require.False(t, capacityQueuedAt(ctx, t, f.db, chat.ID).CapacityQueuedAt.Valid)
}
