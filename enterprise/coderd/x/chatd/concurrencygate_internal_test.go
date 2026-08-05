package chatd

import (
	"context"
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
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func agentHoursSet(t *testing.T, feature codersdk.Feature) *entitlements.Set {
	t.Helper()
	set := entitlements.New()
	set.Modify(func(entitlements *codersdk.Entitlements) {
		entitlements.HasLicense = true
		entitlements.Features[codersdk.FeatureAgentRuntimeHours] = feature
	})
	return set
}

func entitledSet(t *testing.T) *entitlements.Set {
	return agentHoursSet(t, codersdk.Feature{
		Entitlement: codersdk.EntitlementEntitled,
		Enabled:     true,
	})
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

	gate1 := newGate(gateOptions{Store: f.db, Pubsub: f.ps, Capacity: 2, Logger: testutil.Logger(t)})
	gate2 := newGate(gateOptions{Store: f.db, Pubsub: f.ps, Capacity: 2, Logger: testutil.Logger(t)})

	chat1 := f.chat(t)
	chat2 := f.chat(t)
	chat3 := f.chat(t)

	require.NoError(t, gate1.Acquire(ctx, chat1.ID, uuid.Nil))
	require.NoError(t, gate2.Acquire(ctx, chat2.ID, uuid.Nil))

	blockedCtx, cancel := context.WithTimeout(ctx, testutil.IntervalMedium)
	defer cancel()
	err := gate2.Acquire(blockedCtx, chat3.ID, uuid.Nil)
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
			if err := g.Acquire(claimCtx, chats[i].ID, uuid.Nil); err == nil {
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

	// The mock clock never advances, so admission must come from pubsub.
	clock := quartz.NewMock(t)
	g := newGate(gateOptions{Store: f.db, Pubsub: f.ps, Capacity: 1, Clock: clock, Logger: testutil.Logger(t)})

	active := f.chat(t)
	queued := f.chat(t)
	require.NoError(t, g.Acquire(ctx, active.ID, uuid.Nil))

	admitted := make(chan error, 1)
	go func() {
		admitted <- g.Acquire(ctx, queued.ID, uuid.Nil)
	}()

	require.Eventually(t, func() bool {
		state := concurrencyStateOf(ctx, t, f.db, queued.ID)
		return state.Valid && state.ChatConcurrencyState == database.ChatConcurrencyStateQueued
	}, testutil.WaitLong, testutil.IntervalFast)

	// The frozen mock clock proves the queue timestamp comes from the
	// database clock, not the replica clock.
	queuedRow, err := f.db.GetChatByID(ctx, queued.ID)
	require.NoError(t, err)
	require.True(t, queuedRow.ConcurrencyQueuedAt.Valid)
	require.WithinDuration(t, time.Now(), queuedRow.ConcurrencyQueuedAt.Time, time.Minute)

	// ChatMachine.Update publishes the nudge after FinishTurn commits.
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
	require.NoError(t, g.Acquire(ctx, active.ID, uuid.Nil))

	admitted := make(chan error, 1)
	go func() {
		admitted <- g.Acquire(ctx, queued.ID, uuid.Nil)
	}()

	require.Eventually(t, func() bool {
		state := concurrencyStateOf(ctx, t, f.db, queued.ID)
		return state.Valid && state.ChatConcurrencyState == database.ChatConcurrencyStateQueued
	}, testutil.WaitLong, testutil.IntervalFast)

	// Free the slot without publishing a capacity nudge.
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
	require.NoError(t, g.Acquire(ctx, active.ID, uuid.Nil))

	olderAdmitted := make(chan error, 1)
	go func() {
		olderAdmitted <- g.Acquire(ctx, older.ID, uuid.Nil)
	}()
	require.Eventually(t, func() bool {
		state := concurrencyStateOf(ctx, t, f.db, older.ID)
		return state.Valid && state.ChatConcurrencyState == database.ChatConcurrencyStateQueued
	}, testutil.WaitLong, testutil.IntervalFast)

	newerAdmitted := make(chan error, 1)
	go func() {
		newerAdmitted <- g.Acquire(ctx, newer.ID, uuid.Nil)
	}()
	require.Eventually(t, func() bool {
		state := concurrencyStateOf(ctx, t, f.db, newer.ID)
		return state.Valid && state.ChatConcurrencyState == database.ChatConcurrencyStateQueued
	}, testutil.WaitLong, testutil.IntervalFast)

	// The nudge wakes both waiters, but only the oldest may claim the slot.
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

func TestGateRuntimeHoursBypass(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		feature codersdk.Feature
	}{
		{
			name: "UsageUnknown",
			feature: codersdk.Feature{
				Entitlement: codersdk.EntitlementEntitled,
				Enabled:     true,
			},
		},
		{
			name: "RemainingHours",
			feature: codersdk.Feature{
				Entitlement: codersdk.EntitlementEntitled,
				Enabled:     true,
				Limit:       ptr.Ref(int64(100)),
				Actual:      ptr.Ref(int64(99)),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newGateFixture(t)
			ctx := testutil.Context(t, testutil.WaitLong)

			g := newGate(gateOptions{Entitlements: agentHoursSet(t, tc.feature), Store: f.db, Pubsub: f.ps, Capacity: 1, Logger: testutil.Logger(t)})

			for range 3 {
				chat := f.chat(t)
				require.NoError(t, g.Acquire(ctx, chat.ID, uuid.Nil))
				state := concurrencyStateOf(ctx, t, f.db, chat.ID)
				require.False(t, state.Valid)
			}
		})
	}
}

func TestGateEntitledBypassClearsStaleMarker(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	admittedChats := make(chan database.Chat, 1)
	g := newGate(gateOptions{
		Entitlements: entitledSet(t),
		Store:        f.db,
		Pubsub:       f.ps,
		Capacity:     1,
		Logger:       testutil.Logger(t),
		OnAdmitted: func(chat database.Chat) {
			admittedChats <- chat
		},
	})

	// A queued marker left behind by a canceled waiter, e.g. a runner
	// replaced before the entitlement was installed.
	stale := f.chat(t)
	_, err := f.db.SetChatConcurrencyState(ctx, database.SetChatConcurrencyStateParams{
		ID: stale.ID,
		ConcurrencyState: database.NullChatConcurrencyState{
			ChatConcurrencyState: database.ChatConcurrencyStateQueued,
			Valid:                true,
		},
	})
	require.NoError(t, err)

	require.NoError(t, g.Acquire(ctx, stale.ID, uuid.Nil))
	state := concurrencyStateOf(ctx, t, f.db, stale.ID)
	require.False(t, state.Valid)
	admitted := testutil.RequireReceive(ctx, t, admittedChats)
	require.Equal(t, stale.ID, admitted.ID)

	// Unmarked chats admit without publishing.
	clean := f.chat(t)
	require.NoError(t, g.Acquire(ctx, clean.ID, uuid.Nil))
	require.Len(t, admittedChats, 0)
}

func TestGateLicensedCapped(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		feature codersdk.Feature
	}{
		{
			name: "WithoutHours",
			feature: codersdk.Feature{
				Entitlement: codersdk.EntitlementEntitled,
				Enabled:     false,
			},
		},
		{
			name: "ExhaustedHours",
			feature: codersdk.Feature{
				Entitlement: codersdk.EntitlementEntitled,
				Enabled:     true,
				Limit:       ptr.Ref(int64(100)),
				Actual:      ptr.Ref(int64(100)),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newGateFixture(t)
			ctx := testutil.Context(t, testutil.WaitLong)

			g := newGate(gateOptions{Entitlements: agentHoursSet(t, tc.feature), Store: f.db, Pubsub: f.ps, Capacity: 1, Logger: testutil.Logger(t)})

			first := f.chat(t)
			second := f.chat(t)
			require.NoError(t, g.Acquire(ctx, first.ID, uuid.Nil))

			blockedCtx, cancel := context.WithTimeout(ctx, testutil.IntervalMedium)
			defer cancel()
			require.ErrorIs(t, g.Acquire(blockedCtx, second.ID, uuid.Nil), context.DeadlineExceeded)

			state := concurrencyStateOf(ctx, t, f.db, second.ID)
			require.True(t, state.Valid)
			require.Equal(t, database.ChatConcurrencyStateQueued, state.ChatConcurrencyState)
		})
	}
}

// testGateAdmitsAfterFeatureChange queues a chat behind a full gate, then
// swaps the agent runtime hours feature and expects the waiter to admit.
func testGateAdmitsAfterFeatureChange(t *testing.T, set *entitlements.Set, updated codersdk.Feature) {
	t.Helper()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

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
	require.NoError(t, g.Acquire(ctx, active.ID, uuid.Nil))

	admitted := make(chan error, 1)
	go func() {
		admitted <- g.Acquire(ctx, queued.ID, uuid.Nil)
	}()
	require.Eventually(t, func() bool {
		state := concurrencyStateOf(ctx, t, f.db, queued.ID)
		return state.Valid && state.ChatConcurrencyState == database.ChatConcurrencyStateQueued
	}, testutil.WaitLong, testutil.IntervalFast)

	set.Modify(func(entitlements *codersdk.Entitlements) {
		entitlements.Features[codersdk.FeatureAgentRuntimeHours] = updated
	})
	require.NoError(t, testutil.RequireReceive(ctx, t, admitted))
	state := concurrencyStateOf(ctx, t, f.db, queued.ID)
	require.False(t, state.Valid)
}

func TestGateEntitlementInstalledWhileQueued(t *testing.T) {
	t.Parallel()
	testGateAdmitsAfterFeatureChange(t, entitlements.New(), codersdk.Feature{
		Entitlement: codersdk.EntitlementEntitled,
		Enabled:     true,
	})
}

func TestGateHoursRenewedWhileQueued(t *testing.T) {
	t.Parallel()
	exhausted := agentHoursSet(t, codersdk.Feature{
		Entitlement: codersdk.EntitlementEntitled,
		Enabled:     true,
		Limit:       ptr.Ref(int64(100)),
		Actual:      ptr.Ref(int64(100)),
	})
	testGateAdmitsAfterFeatureChange(t, exhausted, codersdk.Feature{
		Entitlement: codersdk.EntitlementEntitled,
		Enabled:     true,
		Limit:       ptr.Ref(int64(100)),
		Actual:      ptr.Ref(int64(0)),
	})
}

func TestGateYieldFreesCapacity(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	clock := quartz.NewMock(t)
	g := newGate(gateOptions{Store: f.db, Pubsub: f.ps, Capacity: 1, Clock: clock, Logger: testutil.Logger(t)})

	parent := f.chat(t)
	child := f.chat(t)
	require.NoError(t, g.Acquire(ctx, parent.ID, uuid.Nil))

	blockedCtx, cancel := context.WithTimeout(ctx, testutil.IntervalMedium)
	err := g.Acquire(blockedCtx, child.ID, uuid.Nil)
	cancel()
	require.ErrorIs(t, err, context.DeadlineExceeded)

	// Yield publishes a nudge so the child can claim the freed slot.
	require.NoError(t, g.Yield(ctx, parent.ID, uuid.Nil))
	state := concurrencyStateOf(ctx, t, f.db, parent.ID)
	require.True(t, state.Valid)
	require.Equal(t, database.ChatConcurrencyStateYielded, state.ChatConcurrencyState)
	require.NoError(t, g.Acquire(ctx, child.ID, uuid.Nil))

	resumed := make(chan error, 1)
	go func() {
		resumed <- g.Acquire(ctx, parent.ID, uuid.Nil)
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

func TestGateWritesFencedToRunner(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	g := newGate(gateOptions{Store: f.db, Pubsub: f.ps, Capacity: 1, Logger: testutil.Logger(t)})

	owner := uuid.New()
	stale := uuid.New()
	chat := f.chat(t)
	_, err := f.db.UpdateChatExecutionState(ctx, database.UpdateChatExecutionStateParams{
		ID:       chat.ID,
		Status:   database.ChatStatusRunning,
		WorkerID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
		RunnerID: uuid.NullUUID{UUID: owner, Valid: true},
	})
	require.NoError(t, err)
	require.NoError(t, g.Acquire(ctx, chat.ID, owner))

	// A stale runner surviving heartbeat recovery must not overwrite the
	// owner's active claim with yielded.
	require.NoError(t, g.Yield(ctx, chat.ID, stale))
	state := concurrencyStateOf(ctx, t, f.db, chat.ID)
	require.True(t, state.Valid)
	require.Equal(t, database.ChatConcurrencyStateActive, state.ChatConcurrencyState)

	// A stale acquire admits without writing markers.
	require.NoError(t, g.Acquire(ctx, chat.ID, stale))
	state = concurrencyStateOf(ctx, t, f.db, chat.ID)
	require.Equal(t, database.ChatConcurrencyStateActive, state.ChatConcurrencyState)

	// The owning runner's yield still works.
	require.NoError(t, g.Yield(ctx, chat.ID, owner))
	state = concurrencyStateOf(ctx, t, f.db, chat.ID)
	require.True(t, state.Valid)
	require.Equal(t, database.ChatConcurrencyStateYielded, state.ChatConcurrencyState)
}

func TestAutoArchiveClearsCapacityMarkers(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	root := f.chat(t)
	_, err := f.db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:     root.ID,
		Status: database.ChatStatusWaiting,
	})
	require.NoError(t, err)
	child := dbgen.Chat(t, f.db, database.Chat{
		OwnerID:           f.owner.ID,
		OrganizationID:    f.org.ID,
		LastModelConfigID: f.modelConfig.ID,
		Status:            database.ChatStatusRunning,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})
	_, err = f.db.SetChatConcurrencyState(ctx, database.SetChatConcurrencyStateParams{
		ID: child.ID,
		ConcurrencyState: database.NullChatConcurrencyState{
			ChatConcurrencyState: database.ChatConcurrencyStateActive,
			Valid:                true,
		},
	})
	require.NoError(t, err)

	// A future cutoff makes the just-created inactive root eligible; the
	// archive cascades to the running child.
	rows, err := f.db.AutoArchiveInactiveChats(ctx, database.AutoArchiveInactiveChatsParams{
		ArchiveCutoff: time.Now().Add(time.Hour),
		LimitCount:    10,
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)

	archived, err := f.db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.True(t, archived.Archived)
	require.False(t, archived.ConcurrencyState.Valid)
	require.False(t, archived.ConcurrencyQueuedAt.Valid)
}

func TestGateQueuedAtSurvivesInterrupt(t *testing.T) {
	t.Parallel()
	f := newGateFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	chat := f.chat(t)
	seeded, err := f.db.SetChatConcurrencyState(ctx, database.SetChatConcurrencyStateParams{
		ID: chat.ID,
		ConcurrencyState: database.NullChatConcurrencyState{
			ChatConcurrencyState: database.ChatConcurrencyStateQueued,
			Valid:                true,
		},
	})
	require.NoError(t, err)
	require.True(t, seeded.ConcurrencyQueuedAt.Valid)
	queuedAt := seeded.ConcurrencyQueuedAt.Time

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
	require.NoError(t, g.Acquire(ctx, active.ID, uuid.Nil))

	admitted := make(chan error, 1)
	go func() {
		admitted <- g.Acquire(ctx, queued.ID, uuid.Nil)
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
	require.NoError(t, g.Acquire(ctx, chat.ID, uuid.Nil))
	require.NoError(t, g.Acquire(ctx, chat.ID, uuid.Nil))

	count, err := f.db.CountActiveConcurrencyChats(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

func TestMaxConcurrentAgents(t *testing.T) {
	t.Parallel()
	// This value is part of license enforcement.
	require.Equal(t, int64(5), int64(MaxConcurrentAgents))
}
