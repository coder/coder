package connectionlog

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func Test_addToBatch(t *testing.T) {
	t.Parallel()

	t.Run("ConnectThenDisconnect", func(t *testing.T) {
		t.Parallel()

		b := &DBBatcher{
			maxBatchSize: 100,
			dedupedBatch: make(map[uuid.UUID]batchEntry),
		}

		wsID := uuid.New()
		connID := uuid.New()

		connect := fakeConnectEvent(wsID, "agent1", connID)
		disconnect := fakeDisconnectEvent(wsID, "agent1", connID)

		b.addToBatch(connect)
		b.addToBatch(disconnect)

		require.Equal(t, 1, b.batchLen())
		key := connID
		got := b.dedupedBatch[key]
		require.Equal(t, disconnect.ID, got.ID)
		require.Equal(t, database.ConnectionStatusDisconnected, got.ConnectionStatus)
		// The connect_time should be preserved from the original
		// connect event, not overwritten by the disconnect's
		// timestamp.
		require.Equal(t, connect.Time, got.connectTime)
		require.Equal(t, disconnect.Time, got.disconnectTime)
	})

	t.Run("DisconnectThenLaterConnect", func(t *testing.T) {
		t.Parallel()

		b := &DBBatcher{
			maxBatchSize: 100,
			dedupedBatch: make(map[uuid.UUID]batchEntry),
		}

		wsID := uuid.New()
		connID := uuid.New()

		disconnect := fakeDisconnectEvent(wsID, "agent1", connID)
		connect := fakeConnectEvent(wsID, "agent1", connID)
		connect.Time = disconnect.Time.Add(time.Second)

		b.addToBatch(disconnect)
		b.addToBatch(connect)

		require.Equal(t, 1, b.batchLen())
		key := connID
		// The later event wins when the incoming item is not a
		// disconnect. In practice, this case doesn't occur because
		// connection IDs are never reused.
		require.Equal(t, connect.ID, b.dedupedBatch[key].ID)
	})

	t.Run("DisconnectThenEarlierConnect", func(t *testing.T) {
		t.Parallel()

		b := &DBBatcher{
			maxBatchSize: 100,
			dedupedBatch: make(map[uuid.UUID]batchEntry),
		}

		wsID := uuid.New()
		connID := uuid.New()

		disconnect := fakeDisconnectEvent(wsID, "agent1", connID)
		connect := fakeConnectEvent(wsID, "agent1", connID)
		connect.Time = disconnect.Time.Add(-time.Second)

		b.addToBatch(disconnect)
		b.addToBatch(connect)

		require.Equal(t, 1, b.batchLen())
		key := connID
		require.Equal(t, disconnect.ID, b.dedupedBatch[key].ID)
	})

	t.Run("SameStatusKeepsLater", func(t *testing.T) {
		t.Parallel()

		b := &DBBatcher{
			maxBatchSize: 100,
			dedupedBatch: make(map[uuid.UUID]batchEntry),
		}

		wsID := uuid.New()
		connID := uuid.New()

		early := fakeConnectEvent(wsID, "agent1", connID)
		early.Time = time.Now()
		late := fakeConnectEvent(wsID, "agent1", connID)
		late.Time = early.Time.Add(time.Second)

		b.addToBatch(early)
		b.addToBatch(late)

		require.Equal(t, 1, b.batchLen())
		key := connID
		require.Equal(t, late.ID, b.dedupedBatch[key].ID)
	})

	t.Run("NullConnIDsNeverDedup", func(t *testing.T) {
		t.Parallel()

		b := &DBBatcher{
			maxBatchSize: 100,
			dedupedBatch: make(map[uuid.UUID]batchEntry),
		}

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

		b := &DBBatcher{
			maxBatchSize: 100,
			dedupedBatch: make(map[uuid.UUID]batchEntry),
		}

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
		Type:             database.ConnectionTypeSsh,
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
		Type:             database.ConnectionTypeSsh,
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
		Type:             database.ConnectionTypeWorkspaceApp,
		ConnectionID:     uuid.NullUUID{},
		ConnectionStatus: database.ConnectionStatusConnected,
	}
}
