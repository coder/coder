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

// connLogParamsMatcher validates BatchUpsertConnectionLogsParams by
// checking that the expected IDs are present (order-independent).
type connLogParamsMatcher struct {
	expectedIDs []uuid.UUID
}

func (m connLogParamsMatcher) Matches(x interface{}) bool {
	params, ok := x.(database.BatchUpsertConnectionLogsParams)
	if !ok {
		return false
	}
	if len(params.ID) != len(m.expectedIDs) {
		return false
	}
	found := make(map[uuid.UUID]bool, len(m.expectedIDs))
	for _, id := range m.expectedIDs {
		found[id] = false
	}
	for _, id := range params.ID {
		if _, exists := found[id]; !exists {
			return false
		}
		found[id] = true
	}
	for _, v := range found {
		if !v {
			return false
		}
	}
	return true
}

func (m connLogParamsMatcher) String() string {
	return "batch upsert params matching expected IDs"
}

func matchIDs(ids ...uuid.UUID) gomock.Matcher {
	return connLogParamsMatcher{expectedIDs: ids}
}

// connLogCountMatcher validates that the batch has the expected number
// of entries.
type connLogCountMatcher struct {
	expectedCount int
}

func (m connLogCountMatcher) Matches(x interface{}) bool {
	params, ok := x.(database.BatchUpsertConnectionLogsParams)
	if !ok {
		return false
	}
	return len(params.ID) == m.expectedCount
}

func (m connLogCountMatcher) String() string {
	return "batch upsert params with expected count"
}

func matchCount(n int) gomock.Matcher {
	return connLogCountMatcher{expectedCount: n}
}

func newConnectEvent(workspaceID uuid.UUID, agentName string, connectionID uuid.UUID) database.UpsertConnectionLogParams {
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

func newDisconnectEvent(workspaceID uuid.UUID, agentName string, connectionID uuid.UUID) database.UpsertConnectionLogParams {
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

func newNullConnIDEvent() database.UpsertConnectionLogParams {
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

func TestDBBackend(t *testing.T) {
	t.Parallel()

	t.Run("DeduplicateConnectDisconnect", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)

		b := NewDBBackend(ctx, store, log, WithClock(clock), WithBatchSize(100))

		wsID := uuid.New()
		connID := uuid.New()
		agentName := "agent1"

		connect := newConnectEvent(wsID, agentName, connID)
		disconnect := newDisconnectEvent(wsID, agentName, connID)

		// Expect only 1 entry (the disconnect, which wins).
		store.EXPECT().
			BatchUpsertConnectionLogs(gomock.Any(), matchIDs(disconnect.ID)).
			Return(nil).
			Times(1)

		err := b.Upsert(ctx, connect)
		require.NoError(t, err)
		err = b.Upsert(ctx, disconnect)
		require.NoError(t, err)

		// Close drains the channel and flushes, guaranteeing both
		// events are in the batch.
		err = b.Close()
		require.NoError(t, err)
	})

	t.Run("NullConnIDNotDeduplicated", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)

		b := NewDBBackend(ctx, store, log, WithClock(clock), WithBatchSize(100))

		evt1 := newNullConnIDEvent()
		evt2 := newNullConnIDEvent()
		// Give them the same workspace/agent to prove they still
		// aren't deduped.
		evt2.WorkspaceID = evt1.WorkspaceID
		evt2.AgentName = evt1.AgentName

		store.EXPECT().
			BatchUpsertConnectionLogs(gomock.Any(), matchIDs(evt1.ID, evt2.ID)).
			Return(nil).
			Times(1)

		err := b.Upsert(ctx, evt1)
		require.NoError(t, err)
		err = b.Upsert(ctx, evt2)
		require.NoError(t, err)

		err = b.Close()
		require.NoError(t, err)
	})

	t.Run("DifferentConnectionIDsNotDeduplicated", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)

		b := NewDBBackend(ctx, store, log, WithClock(clock), WithBatchSize(100))

		wsID := uuid.New()
		agentName := "agent1"
		evt1 := newConnectEvent(wsID, agentName, uuid.New())
		evt2 := newConnectEvent(wsID, agentName, uuid.New())

		store.EXPECT().
			BatchUpsertConnectionLogs(gomock.Any(), matchIDs(evt1.ID, evt2.ID)).
			Return(nil).
			Times(1)

		err := b.Upsert(ctx, evt1)
		require.NoError(t, err)
		err = b.Upsert(ctx, evt2)
		require.NoError(t, err)

		err = b.Close()
		require.NoError(t, err)
	})

	t.Run("CloseFlushesRemaining", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		clock := quartz.NewMock(t)

		b := NewDBBackend(ctx, store, log, WithClock(clock), WithBatchSize(100))

		event := newConnectEvent(uuid.New(), "agent1", uuid.New())

		store.EXPECT().
			BatchUpsertConnectionLogs(gomock.Any(), matchIDs(event.ID)).
			Return(nil).
			Times(1)

		err := b.Upsert(ctx, event)
		require.NoError(t, err)

		// Close should trigger final flush without advancing the
		// clock.
		err = b.Close()
		require.NoError(t, err)
	})




}

func TestAddToBatch(t *testing.T) {
	t.Parallel()

	t.Run("ConnectThenDisconnectKeepsDisconnect", func(t *testing.T) {
		t.Parallel()

		b := &DBBackend{
			dedupedBatch: make(map[conflictKey]database.UpsertConnectionLogParams),
		}

		wsID := uuid.New()
		connID := uuid.New()
		agentName := "agent1"

		connect := newConnectEvent(wsID, agentName, connID)
		disconnect := newDisconnectEvent(wsID, agentName, connID)

		b.addToBatch(connect)
		b.addToBatch(disconnect)

		require.Equal(t, 1, b.batchLen())
		key := conflictKey{
			ConnectionID: connID,
			WorkspaceID:  wsID,
			AgentName:    agentName,
		}
		require.Equal(t, disconnect.ID, b.dedupedBatch[key].ID)
		require.Equal(t, database.ConnectionStatusDisconnected, b.dedupedBatch[key].ConnectionStatus)
	})

	t.Run("DisconnectThenLaterConnectKeepsLater", func(t *testing.T) {
		t.Parallel()

		b := &DBBackend{
			dedupedBatch: make(map[conflictKey]database.UpsertConnectionLogParams),
		}

		wsID := uuid.New()
		connID := uuid.New()
		agentName := "agent1"

		// Disconnect arrives first (out of order).
		disconnect := newDisconnectEvent(wsID, agentName, connID)
		connect := newConnectEvent(wsID, agentName, connID)
		// Make connect later in time.
		connect.Time = disconnect.Time.Add(time.Second)

		b.addToBatch(disconnect)
		b.addToBatch(connect)

		require.Equal(t, 1, b.batchLen())
		key := conflictKey{
			ConnectionID: connID,
			WorkspaceID:  wsID,
			AgentName:    agentName,
		}
		// The later event wins when the incoming item is not a
		// disconnect. In practice, this case doesn't occur because
		// connection IDs are never reused.
		require.Equal(t, connect.ID, b.dedupedBatch[key].ID)
	})

	t.Run("DisconnectThenEarlierConnectKeepsDisconnect", func(t *testing.T) {
		t.Parallel()

		b := &DBBackend{
			dedupedBatch: make(map[conflictKey]database.UpsertConnectionLogParams),
		}

		wsID := uuid.New()
		connID := uuid.New()
		agentName := "agent1"

		disconnect := newDisconnectEvent(wsID, agentName, connID)
		connect := newConnectEvent(wsID, agentName, connID)
		// Make connect earlier in time than disconnect.
		connect.Time = disconnect.Time.Add(-time.Second)

		b.addToBatch(disconnect)
		b.addToBatch(connect)

		require.Equal(t, 1, b.batchLen())
		key := conflictKey{
			ConnectionID: connID,
			WorkspaceID:  wsID,
			AgentName:    agentName,
		}
		// Disconnect stays because connect is not later.
		require.Equal(t, disconnect.ID, b.dedupedBatch[key].ID)
	})

	t.Run("SameStatusKeepsLater", func(t *testing.T) {
		t.Parallel()

		b := &DBBackend{
			dedupedBatch: make(map[conflictKey]database.UpsertConnectionLogParams),
		}

		wsID := uuid.New()
		connID := uuid.New()
		agentName := "agent1"

		early := newConnectEvent(wsID, agentName, connID)
		early.Time = time.Now()

		late := newConnectEvent(wsID, agentName, connID)
		late.Time = early.Time.Add(time.Second)

		b.addToBatch(early)
		b.addToBatch(late)

		require.Equal(t, 1, b.batchLen())
		key := conflictKey{
			ConnectionID: connID,
			WorkspaceID:  wsID,
			AgentName:    agentName,
		}
		require.Equal(t, late.ID, b.dedupedBatch[key].ID)
	})

	t.Run("NullConnIDsNeverDedup", func(t *testing.T) {
		t.Parallel()

		b := &DBBackend{
			dedupedBatch: make(map[conflictKey]database.UpsertConnectionLogParams),
		}

		evt1 := newNullConnIDEvent()
		evt2 := newNullConnIDEvent()
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

		b := &DBBackend{
			dedupedBatch: make(map[conflictKey]database.UpsertConnectionLogParams),
		}

		wsID := uuid.New()
		agentName := "agent1"

		regular := newConnectEvent(wsID, agentName, uuid.New())
		nullEvt := newNullConnIDEvent()
		nullEvt.WorkspaceID = wsID
		nullEvt.AgentName = agentName

		b.addToBatch(regular)
		b.addToBatch(nullEvt)

		require.Equal(t, 2, b.batchLen())
		require.Len(t, b.dedupedBatch, 1)
		require.Len(t, b.nullConnIDBatch, 1)
	})
}
