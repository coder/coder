package chatd

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func entitledSet(t *testing.T) *entitlements.Set {
	t.Helper()
	set := entitlements.New()
	set.Modify(func(entitlements *codersdk.Entitlements) {
		entitlements.Features[codersdk.FeatureAgentRuntimeHours] = codersdk.Feature{
			Entitlement: codersdk.EntitlementEntitled,
			Enabled:     true,
		}
	})
	return set
}

func licensedWithoutHours(t *testing.T) *entitlements.Set {
	t.Helper()
	set := entitlements.New()
	set.Modify(func(entitlements *codersdk.Entitlements) {
		entitlements.HasLicense = true
		entitlements.Features[codersdk.FeatureAgentRuntimeHours] = codersdk.Feature{
			Entitlement: codersdk.EntitlementEntitled,
			Enabled:     false,
		}
	})
	return set
}

type gateFixture struct {
	db          database.Store
	ps          pubsub.Pubsub
	owner       database.User
	org         database.Organization
	modelConfig database.ChatModelConfig
}

func newGateFixture(t *testing.T) gateFixture {
	t.Helper()
	db, ps := dbtestutil.NewDB(t)
	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	return gateFixture{db: db, ps: ps, owner: owner, org: org, modelConfig: modelConfig}
}

func (f gateFixture) chat(t *testing.T) database.Chat {
	t.Helper()
	return dbgen.Chat(t, f.db, database.Chat{
		OwnerID:           f.owner.ID,
		OrganizationID:    f.org.ID,
		LastModelConfigID: f.modelConfig.ID,
		Status:            database.ChatStatusRunning,
	})
}

func concurrencyStateOf(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID) database.NullChatConcurrencyState {
	t.Helper()
	chat, err := db.GetChatByID(ctx, chatID)
	require.NoError(t, err)
	return chat.ConcurrencyState
}

func TestGateCapAcrossInstances(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Two gate instances sharing one database model two replicas.
	gate1 := newGate(gateOptions{Store: f.db, Pubsub: f.ps, Capacity: 2, Logger: testutil.Logger(t)})
	gate2 := newGate(gateOptions{Store: f.db, Pubsub: f.ps, Capacity: 2, Logger: testutil.Logger(t)})

	chat1 := f.chat(t)
	chat2 := f.chat(t)
	chat3 := f.chat(t)

	require.NoError(t, gate1.Acquire(ctx, chat1.ID))
	require.NoError(t, gate2.Acquire(ctx, chat2.ID))

	blockedCtx, cancel := context.WithTimeout(ctx, testutil.IntervalMedium)
	defer cancel()
	err := gate2.Acquire(blockedCtx, chat3.ID)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	state := concurrencyStateOf(ctx, t, f.db, chat3.ID)
	require.True(t, state.Valid)
	require.Equal(t, database.ChatConcurrencyStateQueued, state.ChatConcurrencyState)

	count, err := f.db.CountActiveConcurrencyChats(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}

func TestGateParallelClaimsNeverOverAdmit(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	const capacity = 3
	const claimants = 10
	g := newGate(gateOptions{Store: f.db, Pubsub: f.ps, Capacity: capacity, Logger: testutil.Logger(t)})

	chats := make([]database.Chat, claimants)
	for i := range chats {
		chats[i] = f.chat(t)
	}

	claimCtx, cancelClaims := context.WithCancel(ctx)
	defer cancelClaims()
	var admitted atomic.Int64
	var wg sync.WaitGroup
	for i := range chats {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := g.Acquire(claimCtx, chats[i].ID); err == nil {
				admitted.Add(1)
			}
		}()
	}

	require.Eventually(t, func() bool {
		return admitted.Load() == capacity
	}, testutil.WaitLong, testutil.IntervalFast)
	count, err := f.db.CountActiveConcurrencyChats(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(capacity), count)

	cancelClaims()
	wg.Wait()
	require.Equal(t, int64(capacity), admitted.Load())
}

func TestGateAdmitsOnCapacityNudge(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// A mock clock that is never advanced proves admission arrives
	// through the pubsub nudge, not the fallback poll.
	clock := quartz.NewMock(t)
	g := newGate(gateOptions{Store: f.db, Pubsub: f.ps, Capacity: 1, Clock: clock, Logger: testutil.Logger(t)})

	active := f.chat(t)
	queued := f.chat(t)
	require.NoError(t, g.Acquire(ctx, active.ID))

	admitted := make(chan error, 1)
	go func() {
		admitted <- g.Acquire(ctx, queued.ID)
	}()

	require.Eventually(t, func() bool {
		state := concurrencyStateOf(ctx, t, f.db, queued.ID)
		return state.Valid && state.ChatConcurrencyState == database.ChatConcurrencyStateQueued
	}, testutil.WaitLong, testutil.IntervalFast)

	// The state machine publishes the capacity nudge after the completion
	// transition commits.
	machine := chatstate.NewChatMachine(f.db, f.ps, active.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))

	require.NoError(t, testutil.RequireReceive(ctx, t, admitted))
	state := concurrencyStateOf(ctx, t, f.db, queued.ID)
	require.True(t, state.Valid)
	require.Equal(t, database.ChatConcurrencyStateActive, state.ChatConcurrencyState)
}

type brokenSubscribePubsub struct {
	pubsub.Pubsub
}

func (brokenSubscribePubsub) Subscribe(string, pubsub.Listener) (func(), error) {
	return nil, assert.AnError
}

func TestGateFallbackPollAdmits(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	g := newGate(gateOptions{
		Store:        f.db,
		Pubsub:       brokenSubscribePubsub{f.ps},
		Capacity:     1,
		PollInterval: testutil.IntervalFast,
		Logger:       testutil.Logger(t),
	})

	active := f.chat(t)
	queued := f.chat(t)
	require.NoError(t, g.Acquire(ctx, active.ID))

	admitted := make(chan error, 1)
	go func() {
		admitted <- g.Acquire(ctx, queued.ID)
	}()

	require.Eventually(t, func() bool {
		state := concurrencyStateOf(ctx, t, f.db, queued.ID)
		return state.Valid && state.ChatConcurrencyState == database.ChatConcurrencyStateQueued
	}, testutil.WaitLong, testutil.IntervalFast)

	// Release capacity without any pubsub delivery.
	_, err := f.db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:     active.ID,
		Status: database.ChatStatusWaiting,
	})
	require.NoError(t, err)

	require.NoError(t, testutil.RequireReceive(ctx, t, admitted))
}

func TestGateOldestQueuedFirst(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	clock := quartz.NewMock(t)
	g := newGate(gateOptions{Store: f.db, Pubsub: f.ps, Capacity: 1, Clock: clock, Logger: testutil.Logger(t)})

	active := f.chat(t)
	older := f.chat(t)
	newer := f.chat(t)
	require.NoError(t, g.Acquire(ctx, active.ID))

	olderAdmitted := make(chan error, 1)
	go func() {
		olderAdmitted <- g.Acquire(ctx, older.ID)
	}()
	require.Eventually(t, func() bool {
		state := concurrencyStateOf(ctx, t, f.db, older.ID)
		return state.Valid && state.ChatConcurrencyState == database.ChatConcurrencyStateQueued
	}, testutil.WaitLong, testutil.IntervalFast)

	// Advance the mock clock (within the pending poll timer) so the
	// second chat queues strictly later.
	clock.Advance(time.Second).MustWait(ctx)
	newerAdmitted := make(chan error, 1)
	go func() {
		newerAdmitted <- g.Acquire(ctx, newer.ID)
	}()
	require.Eventually(t, func() bool {
		state := concurrencyStateOf(ctx, t, f.db, newer.ID)
		return state.Valid && state.ChatConcurrencyState == database.ChatConcurrencyStateQueued
	}, testutil.WaitLong, testutil.IntervalFast)

	// Free the slot; the nudge wakes both waiters but only the oldest
	// queued chat may claim it.
	machine := chatstate.NewChatMachine(f.db, f.ps, active.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))

	require.NoError(t, testutil.RequireReceive(ctx, t, olderAdmitted))
	select {
	case err := <-newerAdmitted:
		t.Fatalf("newer chat admitted before older queued chat: %v", err)
	default:
	}
	olderState := concurrencyStateOf(ctx, t, f.db, older.ID)
	require.True(t, olderState.Valid)
	require.Equal(t, database.ChatConcurrencyStateActive, olderState.ChatConcurrencyState)
	newerState := concurrencyStateOf(ctx, t, f.db, newer.ID)
	require.True(t, newerState.Valid)
	require.Equal(t, database.ChatConcurrencyStateQueued, newerState.ChatConcurrencyState)
}

func TestGateEntitledBypass(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	g := newGate(gateOptions{Entitlements: entitledSet(t), Store: f.db, Pubsub: f.ps, Capacity: 1, Logger: testutil.Logger(t)})

	for range 3 {
		chat := f.chat(t)
		require.NoError(t, g.Acquire(ctx, chat.ID))
		state := concurrencyStateOf(ctx, t, f.db, chat.ID)
		require.False(t, state.Valid)
	}
}

func TestGateLicensedWithoutHoursIsCapped(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	g := newGate(gateOptions{Entitlements: licensedWithoutHours(t), Store: f.db, Pubsub: f.ps, Capacity: 1, Logger: testutil.Logger(t)})

	first := f.chat(t)
	second := f.chat(t)
	require.NoError(t, g.Acquire(ctx, first.ID))

	blockedCtx, cancel := context.WithTimeout(ctx, testutil.IntervalMedium)
	defer cancel()
	require.ErrorIs(t, g.Acquire(blockedCtx, second.ID), context.DeadlineExceeded)
}

func TestGateEntitlementInstalledWhileQueued(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	set := entitlements.New()
	g := newGate(gateOptions{
		Entitlements: set,
		Store:        f.db,
		Pubsub:       f.ps,
		Capacity:     1,
		PollInterval: testutil.IntervalFast,
		Logger:       testutil.Logger(t),
	})

	active := f.chat(t)
	queued := f.chat(t)
	require.NoError(t, g.Acquire(ctx, active.ID))

	admitted := make(chan error, 1)
	go func() {
		admitted <- g.Acquire(ctx, queued.ID)
	}()
	require.Eventually(t, func() bool {
		state := concurrencyStateOf(ctx, t, f.db, queued.ID)
		return state.Valid && state.ChatConcurrencyState == database.ChatConcurrencyStateQueued
	}, testutil.WaitLong, testutil.IntervalFast)

	set.Modify(func(entitlements *codersdk.Entitlements) {
		entitlements.Features[codersdk.FeatureAgentRuntimeHours] = codersdk.Feature{
			Entitlement: codersdk.EntitlementEntitled,
			Enabled:     true,
		}
	})
	require.NoError(t, testutil.RequireReceive(ctx, t, admitted))
	state := concurrencyStateOf(ctx, t, f.db, queued.ID)
	require.False(t, state.Valid)
}

func TestGateYieldFreesCapacity(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	clock := quartz.NewMock(t)
	g := newGate(gateOptions{Store: f.db, Pubsub: f.ps, Capacity: 1, Clock: clock, Logger: testutil.Logger(t)})

	parent := f.chat(t)
	child := f.chat(t)
	require.NoError(t, g.Acquire(ctx, parent.ID))

	// The child cannot run while the parent holds the only slot.
	blockedCtx, cancel := context.WithTimeout(ctx, testutil.IntervalMedium)
	err := g.Acquire(blockedCtx, child.ID)
	cancel()
	require.ErrorIs(t, err, context.DeadlineExceeded)

	// Yielding the parent's slot (wait_agent) admits the child via the
	// yield-time nudge.
	require.NoError(t, g.Yield(ctx, parent.ID))
	state := concurrencyStateOf(ctx, t, f.db, parent.ID)
	require.True(t, state.Valid)
	require.Equal(t, database.ChatConcurrencyStateYielded, state.ChatConcurrencyState)
	require.NoError(t, g.Acquire(ctx, child.ID))

	// The parent's resume re-queues behind the child.
	resumed := make(chan error, 1)
	go func() {
		resumed <- g.Acquire(ctx, parent.ID)
	}()
	require.Eventually(t, func() bool {
		state := concurrencyStateOf(ctx, t, f.db, parent.ID)
		return state.Valid && state.ChatConcurrencyState == database.ChatConcurrencyStateQueued
	}, testutil.WaitLong, testutil.IntervalFast)

	machine := chatstate.NewChatMachine(f.db, f.ps, child.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))
	require.NoError(t, testutil.RequireReceive(ctx, t, resumed))
}

func TestGateQueuedAtSurvivesInterrupt(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	chat := f.chat(t)
	queuedAt := time.Now().Add(-time.Minute).UTC().Truncate(time.Millisecond)
	_, err := f.db.SetChatConcurrencyState(ctx, database.SetChatConcurrencyStateParams{
		ID: chat.ID,
		ConcurrencyState: database.NullChatConcurrencyState{
			ChatConcurrencyState: database.ChatConcurrencyStateQueued,
			Valid:                true,
		},
		ConcurrencyQueuedAt: sql.NullTime{Time: queuedAt, Valid: true},
	})
	require.NoError(t, err)

	// running -> interrupting -> running keeps the queue position;
	// leaving the counted statuses clears it.
	for _, status := range []database.ChatStatus{
		database.ChatStatusInterrupting,
		database.ChatStatusRunning,
	} {
		updated, err := f.db.UpdateChatExecutionState(ctx, database.UpdateChatExecutionStateParams{
			ID:     chat.ID,
			Status: status,
		})
		require.NoError(t, err)
		require.True(t, updated.ConcurrencyState.Valid)
		require.Equal(t, database.ChatConcurrencyStateQueued, updated.ConcurrencyState.ChatConcurrencyState)
		require.True(t, updated.ConcurrencyQueuedAt.Valid)
		require.True(t, queuedAt.Equal(updated.ConcurrencyQueuedAt.Time))
	}

	updated, err := f.db.UpdateChatExecutionState(ctx, database.UpdateChatExecutionStateParams{
		ID:     chat.ID,
		Status: database.ChatStatusWaiting,
	})
	require.NoError(t, err)
	require.False(t, updated.ConcurrencyState.Valid)
	require.False(t, updated.ConcurrencyQueuedAt.Valid)
}

func TestGateQueueCallbacks(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	queuedChats := make(chan database.Chat, 1)
	admittedChats := make(chan database.Chat, 1)
	g := newGate(gateOptions{
		Store:    f.db,
		Pubsub:   f.ps,
		Capacity: 1,
		Logger:   testutil.Logger(t),
		OnQueued: func(chat database.Chat) {
			queuedChats <- chat
		},
		OnAdmitted: func(chat database.Chat) {
			admittedChats <- chat
		},
	})

	active := f.chat(t)
	queued := f.chat(t)
	require.NoError(t, g.Acquire(ctx, active.ID))

	admitted := make(chan error, 1)
	go func() {
		admitted <- g.Acquire(ctx, queued.ID)
	}()

	// OnQueued observes the committed queued marker.
	queuedChat := testutil.RequireReceive(ctx, t, queuedChats)
	require.Equal(t, queued.ID, queuedChat.ID)
	require.True(t, queuedChat.ConcurrencyQueuedAt.Valid)

	machine := chatstate.NewChatMachine(f.db, f.ps, active.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))

	require.NoError(t, testutil.RequireReceive(ctx, t, admitted))
	admittedChat := testutil.RequireReceive(ctx, t, admittedChats)
	require.Equal(t, queued.ID, admittedChat.ID)
	require.True(t, admittedChat.ConcurrencyState.Valid)
	require.Equal(t, database.ChatConcurrencyStateActive, admittedChat.ConcurrencyState.ChatConcurrencyState)
}

func TestGateIdempotentAcquire(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	g := newGate(gateOptions{Store: f.db, Pubsub: f.ps, Capacity: 1, Logger: testutil.Logger(t)})

	chat := f.chat(t)
	require.NoError(t, g.Acquire(ctx, chat.ID))
	// Step boundaries and task retries re-acquire; the held slot is
	// re-admitted without waiting even at full capacity.
	require.NoError(t, g.Acquire(ctx, chat.ID))

	count, err := f.db.CountActiveConcurrencyChats(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

func TestMaxConcurrentAgents(t *testing.T) {
	t.Parallel()
	// The product cap is part of the licensing contract; a change must
	// be deliberate.
	require.Equal(t, int64(5), int64(MaxConcurrentAgents))
}
