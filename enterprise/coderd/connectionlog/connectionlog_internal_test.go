package connectionlog

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func Test_addToBatch(t *testing.T) {
	t.Parallel()

	newBatcher := func() *DBBatcher {
		return &DBBatcher{
			maxBatchSize: 100,
			dedupedBatch: make(map[uuid.UUID]batchEntry),
		}
	}

	type event struct {
		disconnect bool
		at         time.Duration
	}
	connect := func(at time.Duration) event { return event{at: at} }
	disconnect := func(at time.Duration) event { return event{disconnect: true, at: at} }
	const noDisconnect = -1

	for _, tc := range []struct {
		name   string
		events []event

		winner, start, end int
		source             database.ConnectionLogConnectTimeSource
	}{
		{"ConnectThenDisconnect", []event{connect(0), disconnect(time.Second)}, 1, 0, 1, database.ConnectionLogConnectTimeSourceConnect},
		{"StandaloneDisconnect", []event{disconnect(0)}, 0, 0, 0, database.ConnectionLogConnectTimeSourceDisconnect},
		{"DisconnectThenLaterConnect", []event{disconnect(0), connect(time.Second)}, 0, 1, 0, database.ConnectionLogConnectTimeSourceConnect},
		{"DisconnectThenEarlierConnect", []event{disconnect(0), connect(-time.Second)}, 0, 1, 0, database.ConnectionLogConnectTimeSourceConnect},
		{"EarlierDisconnectKeepsObservedStart", []event{connect(0), disconnect(-time.Second)}, 1, 0, 1, database.ConnectionLogConnectTimeSourceConnect},
		{"SameStatusKeepsLaterEarliestStart", []event{connect(0), connect(time.Second)}, 1, 0, noDisconnect, database.ConnectionLogConnectTimeSourceConnect},
		{"DuplicateDisconnectsKeepLatest", []event{connect(0), disconnect(time.Second), disconnect(2 * time.Second)}, 2, 0, 2, database.ConnectionLogConnectTimeSourceConnect},
		{"OrphanDuplicateDisconnects", []event{disconnect(0), disconnect(time.Second)}, 1, 0, 1, database.ConnectionLogConnectTimeSourceDisconnect},
		{"OrphanDuplicateDisconnectsThenLaterConnect", []event{disconnect(0), disconnect(time.Second), connect(2 * time.Second)}, 1, 2, 1, database.ConnectionLogConnectTimeSourceConnect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := newBatcher()
			wsID, connID := uuid.New(), uuid.New()
			base := time.Now()
			items := make([]database.UpsertConnectionLogParams, len(tc.events))
			for i, e := range tc.events {
				items[i] = fakeConnectEvent(wsID, "agent1", connID)
				if e.disconnect {
					items[i] = fakeDisconnectEvent(wsID, "agent1", connID)
				}
				items[i].Time = base.Add(e.at)
				b.addToBatch(items[i])
			}

			require.Equal(t, 1, b.batchLen())
			got := b.dedupedBatch[connID]
			winner := items[tc.winner]
			require.Equal(t, winner.ID, got.ID)
			require.Equal(t, winner.ConnectionStatus, got.ConnectionStatus)
			require.Equal(t, winner.Code, got.Code)
			require.Equal(t, winner.DisconnectReason, got.DisconnectReason)
			require.Equal(t, items[tc.start].Time, got.connectTime)
			require.Equal(t, tc.source, got.connectTimeSource)
			if tc.end == noDisconnect {
				require.True(t, got.disconnectTime.IsZero())
			} else {
				require.Equal(t, items[tc.end].Time, got.disconnectTime)
			}
		})
	}

	t.Run("NullConnIDsNeverDedup", func(t *testing.T) {
		t.Parallel()

		b := newBatcher()
		evt1 := fakeNullConnIDEvent()
		evt2 := fakeNullConnIDEvent()
		evt2.WorkspaceID = evt1.WorkspaceID
		evt2.AgentName = evt1.AgentName

		b.addToBatch(evt1)
		b.addToBatch(evt2)

		require.Equal(t, 2, b.batchLen())
		require.Len(t, b.nullConnIDBatch, 2)
		require.Empty(t, b.dedupedBatch)
	})

	t.Run("MixedNullAndNonNull", func(t *testing.T) {
		t.Parallel()

		b := newBatcher()
		wsID := uuid.New()
		regular := fakeConnectEvent(wsID, "agent1", uuid.New())
		nullEvt := fakeNullConnIDEvent()
		nullEvt.WorkspaceID = wsID
		nullEvt.AgentName = "agent1"

		b.addToBatch(regular)
		b.addToBatch(nullEvt)

		require.Equal(t, 2, b.batchLen())
		require.Len(t, b.dedupedBatch, 1)
		require.Len(t, b.nullConnIDBatch, 1)
	})
}

func Test_batcherFlush(t *testing.T) {
	t.Parallel()

	t.Run("DeduplicatesConnectDisconnect", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)

		b := NewDBBatcher(ctx, store, log, WithClock(clock), WithBatchSize(100))

		wsID := uuid.New()
		connID := uuid.New()
		connect := fakeConnectEvent(wsID, "agent1", connID)
		disconnect := fakeDisconnectEvent(wsID, "agent1", connID)

		// Expect a single batch with only the disconnect event.
		store.EXPECT().
			BatchUpsertConnectionLogs(gomock.Any(), batchParamsMatcher{
				expectedCount:     1,
				mustContainIDs:    []uuid.UUID{disconnect.ID},
				mustNotContainIDs: []uuid.UUID{connect.ID},
			}).
			Return(nil).
			Times(1)

		require.NoError(t, b.Upsert(ctx, connect))
		require.NoError(t, b.Upsert(ctx, disconnect))
		require.NoError(t, b.Close())
	})

	t.Run("DoesNotDeduplicateNullConnIDs", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)

		b := NewDBBatcher(ctx, store, log, WithClock(clock), WithBatchSize(100))

		evt1 := fakeNullConnIDEvent()
		evt2 := fakeNullConnIDEvent()
		evt2.WorkspaceID = evt1.WorkspaceID
		evt2.AgentName = evt1.AgentName

		store.EXPECT().
			BatchUpsertConnectionLogs(gomock.Any(), batchParamsMatcher{
				expectedCount:  2,
				mustContainIDs: []uuid.UUID{evt1.ID, evt2.ID},
			}).
			Return(nil).
			Times(1)

		require.NoError(t, b.Upsert(ctx, evt1))
		require.NoError(t, b.Upsert(ctx, evt2))
		require.NoError(t, b.Close())
	})

	t.Run("DoesNotDeduplicateDifferentConnectionIDs", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)

		b := NewDBBatcher(ctx, store, log, WithClock(clock), WithBatchSize(100))

		wsID := uuid.New()
		evt1 := fakeConnectEvent(wsID, "agent1", uuid.New())
		evt2 := fakeConnectEvent(wsID, "agent1", uuid.New())

		store.EXPECT().
			BatchUpsertConnectionLogs(gomock.Any(), batchParamsMatcher{
				expectedCount:  2,
				mustContainIDs: []uuid.UUID{evt1.ID, evt2.ID},
			}).
			Return(nil).
			Times(1)

		require.NoError(t, b.Upsert(ctx, evt1))
		require.NoError(t, b.Upsert(ctx, evt2))
		require.NoError(t, b.Close())
	})

	t.Run("CloseFlushesMultipleEvents", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)

		b := NewDBBatcher(ctx, store, log, WithClock(clock), WithBatchSize(100))

		evt1 := fakeConnectEvent(uuid.New(), "agent1", uuid.New())
		evt2 := fakeConnectEvent(uuid.New(), "agent2", uuid.New())

		store.EXPECT().
			BatchUpsertConnectionLogs(gomock.Any(), batchParamsMatcher{
				expectedCount:  2,
				mustContainIDs: []uuid.UUID{evt1.ID, evt2.ID},
			}).
			Return(nil).
			Times(1)

		require.NoError(t, b.Upsert(ctx, evt1))
		require.NoError(t, b.Upsert(ctx, evt2))
		require.NoError(t, b.Close())
	})

	t.Run("RetriesOnTransientFailure", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)

		// Trap the capacity flush (fires when batch reaches maxBatchSize).
		capacityTrap := clock.Trap().TimerReset("connectionLogBatcher", "capacityFlush")
		defer capacityTrap.Close()

		// Trap the retry backoff timer created by retryBatch.
		retryTrap := clock.Trap().NewTimer("connectionLogBatcher", "retryBackoff")
		defer retryTrap.Close()

		// Batch size of 1: consuming the item triggers an immediate
		// capacity flush, avoiding the timer/itemCh select race.
		b := NewDBBatcher(ctx, store, log, WithClock(clock), WithBatchSize(1))

		evt := fakeConnectEvent(uuid.New(), "agent1", uuid.New())

		gomock.InOrder(
			store.EXPECT().
				BatchUpsertConnectionLogs(gomock.Any(), gomock.Any()).
				Return(xerrors.New("transient error")).
				Times(1),
			store.EXPECT().
				BatchUpsertConnectionLogs(gomock.Any(), batchParamsMatcher{
					expectedCount:  1,
					mustContainIDs: []uuid.UUID{evt.ID},
				}).
				Return(nil).
				Times(1),
		)

		require.NoError(t, b.Upsert(ctx, evt))

		// Item consumed → capacity flush fires → transient error →
		// batch queued to retryCh → timer reset trapped.
		capacityTrap.MustWait(ctx).MustRelease(ctx)

		// Retry worker creates a timer — trap it, release, advance.
		retryCall := retryTrap.MustWait(ctx)
		retryCall.MustRelease(ctx)
		clock.Advance(retryInterval).MustWait(ctx)

		require.NoError(t, b.Close())
	})

	t.Run("ShutdownDrainsRetryQueue", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)

		capacityTrap := clock.Trap().TimerReset("connectionLogBatcher", "capacityFlush")
		defer capacityTrap.Close()

		b := NewDBBatcher(ctx, store, log, WithClock(clock), WithBatchSize(1))

		evt := fakeConnectEvent(uuid.New(), "agent1", uuid.New())

		// Track all successfully written IDs.
		var writtenIDs []uuid.UUID
		var mu sync.Mutex
		firstCall := true
		store.EXPECT().
			BatchUpsertConnectionLogs(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, p database.BatchUpsertConnectionLogsParams) error {
				mu.Lock()
				defer mu.Unlock()
				// First call (synchronous flush) fails, queueing
				// the batch for retry.
				if firstCall {
					firstCall = false
					return xerrors.New("transient error")
				}
				// Drain/retry attempts succeed.
				writtenIDs = append(writtenIDs, p.ID...)
				return nil
			}).
			AnyTimes()

		// Send event — capacity flush triggers immediately.
		require.NoError(t, b.Upsert(ctx, evt))
		capacityTrap.MustWait(ctx).MustRelease(ctx)

		// Close triggers shutdown. The retry worker drains
		// retryCh and writes the batch via writeBatch.
		require.NoError(t, b.Close())

		mu.Lock()
		defer mu.Unlock()
		require.Contains(t, writtenIDs, evt.ID,
			"event should be written during shutdown drain")
	})
}

// batchParamsMatcher validates BatchUpsertConnectionLogsParams by
// checking count and specific IDs.
type batchParamsMatcher struct {
	expectedCount     int
	mustContainIDs    []uuid.UUID
	mustNotContainIDs []uuid.UUID
}

func (m batchParamsMatcher) Matches(x interface{}) bool {
	params, ok := x.(database.BatchUpsertConnectionLogsParams)
	if !ok {
		return false
	}
	if m.expectedCount > 0 && len(params.ID) != m.expectedCount {
		return false
	}
	idSet := make(map[uuid.UUID]struct{}, len(params.ID))
	for _, id := range params.ID {
		idSet[id] = struct{}{}
	}
	for _, id := range m.mustContainIDs {
		if _, ok := idSet[id]; !ok {
			return false
		}
	}
	for _, id := range m.mustNotContainIDs {
		if _, ok := idSet[id]; ok {
			return false
		}
	}
	return true
}

func (batchParamsMatcher) String() string {
	return "batch upsert params matcher"
}

func fakeConnectEvent(workspaceID uuid.UUID, agentName string, connectionID uuid.UUID) database.UpsertConnectionLogParams {
	return database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             time.Now(),
		OrganizationID:   uuid.New(),
		WorkspaceOwnerID: uuid.New(),
		WorkspaceID:      workspaceID,
		WorkspaceName:    "test-workspace",
		AgentName:        agentName,
		ConnectionMethod: database.ConnectionLogMethodSSH,
		ConnectionID:     uuid.NullUUID{UUID: connectionID, Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	}
}

func fakeDisconnectEvent(workspaceID uuid.UUID, agentName string, connectionID uuid.UUID) database.UpsertConnectionLogParams {
	return database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             time.Now().Add(time.Second),
		OrganizationID:   uuid.New(),
		WorkspaceOwnerID: uuid.New(),
		WorkspaceID:      workspaceID,
		WorkspaceName:    "test-workspace",
		AgentName:        agentName,
		ConnectionMethod: database.ConnectionLogMethodSSH,
		ConnectionID:     uuid.NullUUID{UUID: connectionID, Valid: true},
		ConnectionStatus: database.ConnectionStatusDisconnected,
		Code:             sql.NullInt32{Int32: 0, Valid: true},
		DisconnectReason: sql.NullString{String: "normal", Valid: true},
	}
}

func fakeNullConnIDEvent() database.UpsertConnectionLogParams {
	return database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             time.Now(),
		OrganizationID:   uuid.New(),
		WorkspaceOwnerID: uuid.New(),
		WorkspaceID:      uuid.New(),
		WorkspaceName:    "test-workspace",
		AgentName:        "test-agent",
		ConnectionMethod: database.ConnectionLogMethodWorkspaceApp,
		ConnectionID:     uuid.NullUUID{},
		ConnectionStatus: database.ConnectionStatusConnected,
	}
}
