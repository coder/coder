package connectionlog_test

import (
	"database/sql"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/enterprise/coderd/connectionlog"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func createWorkspace(t *testing.T, db database.Store) database.WorkspaceTable {
	t.Helper()
	u := dbgen.User(t, db, database.User{})
	o := dbgen.Organization(t, db, database.Organization{})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: o.ID,
		CreatedBy:      u.ID,
	})
	return dbgen.Workspace(t, db, database.WorkspaceTable{
		ID:               uuid.New(),
		OwnerID:          u.ID,
		OrganizationID:   o.ID,
		AutomaticUpdates: database.AutomaticUpdatesNever,
		TemplateID:       tpl.ID,
	})
}

func testIP() pqtype.Inet {
	return pqtype.Inet{
		IPNet: net.IPNet{
			IP:   net.IPv4(127, 0, 0, 1),
			Mask: net.IPv4Mask(255, 255, 255, 255),
		},
		Valid: true,
	}
}

func TestDBBackendIntegration(t *testing.T) {
	t.Parallel()

	t.Run("SingleConnect", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		clock := quartz.NewMock(t)

		ws := createWorkspace(t, db)

		//nolint:gocritic // Test needs system context for the batcher.
		backend := connectionlog.NewDBBatcher(
			dbauthz.AsConnectionLogger(ctx), db, log,
			connectionlog.WithClock(clock),
			connectionlog.WithBatchSize(100),
		)

		connID := uuid.New()
		connectTime := dbtime.Now()
		err := backend.Upsert(ctx, database.UpsertConnectionLogParams{
			ID:               uuid.New(),
			Time:             connectTime,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceID:      ws.ID,
			WorkspaceName:    ws.Name,
			AgentName:        "main",
			Type:             database.ConnectionTypeSsh,
			ConnectionID:     uuid.NullUUID{UUID: connID, Valid: true},
			ConnectionStatus: database.ConnectionStatusConnected,
			IP:               testIP(),
		})
		require.NoError(t, err)

		err = backend.Close()
		require.NoError(t, err)

		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
			LimitOpt: 10,
		})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, connID, rows[0].ConnectionLog.ConnectionID.UUID)
		require.False(t, rows[0].ConnectionLog.DisconnectTime.Valid)
	})

	t.Run("ConnectThenDisconnectSeparateBatches", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		clock := quartz.NewMock(t)

		ws := createWorkspace(t, db)

		connID := uuid.New()
		connectTime := dbtime.Now()

		// First batcher: insert connect, close to flush.
		//nolint:gocritic // Test needs system context for the batcher.
		b1 := connectionlog.NewDBBatcher(
			dbauthz.AsConnectionLogger(ctx), db, log,
			connectionlog.WithClock(clock),
			connectionlog.WithBatchSize(100),
		)
		err := b1.Upsert(ctx, database.UpsertConnectionLogParams{
			ID:               uuid.New(),
			Time:             connectTime,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceID:      ws.ID,
			WorkspaceName:    ws.Name,
			AgentName:        "main",
			Type:             database.ConnectionTypeSsh,
			ConnectionID:     uuid.NullUUID{UUID: connID, Valid: true},
			ConnectionStatus: database.ConnectionStatusConnected,
			IP:               testIP(),
		})
		require.NoError(t, err)
		require.NoError(t, b1.Close())

		// Second batcher: insert disconnect, close to flush.
		//nolint:gocritic // Test needs system context for the batcher.
		b2 := connectionlog.NewDBBatcher(
			dbauthz.AsConnectionLogger(ctx), db, log,
			connectionlog.WithClock(clock),
			connectionlog.WithBatchSize(100),
		)
		disconnectTime := connectTime.Add(5 * time.Second)
		err = b2.Upsert(ctx, database.UpsertConnectionLogParams{
			ID:               uuid.New(),
			Time:             disconnectTime,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceID:      ws.ID,
			WorkspaceName:    ws.Name,
			AgentName:        "main",
			Type:             database.ConnectionTypeSsh,
			ConnectionID:     uuid.NullUUID{UUID: connID, Valid: true},
			ConnectionStatus: database.ConnectionStatusDisconnected,
			Code:             sql.NullInt32{Int32: 0, Valid: true},
			DisconnectReason: sql.NullString{String: "client left", Valid: true},
			IP:               testIP(),
		})
		require.NoError(t, err)
		require.NoError(t, b2.Close())

		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
			LimitOpt: 10,
		})
		require.NoError(t, err)
		require.Len(t, rows, 1, "connect+disconnect should produce one row")
		require.True(t, rows[0].ConnectionLog.DisconnectTime.Valid)
		require.Equal(t, "client left", rows[0].ConnectionLog.DisconnectReason.String)
	})

	t.Run("ConnectAndDisconnectSameBatch", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		clock := quartz.NewMock(t)

		ws := createWorkspace(t, db)

		//nolint:gocritic // Test needs system context for the batcher.
		backend := connectionlog.NewDBBatcher(
			dbauthz.AsConnectionLogger(ctx), db, log,
			connectionlog.WithClock(clock),
			connectionlog.WithBatchSize(100),
		)

		connID := uuid.New()
		connectTime := dbtime.Now()
		disconnectTime := connectTime.Add(time.Second)

		// Both events in the same batch window.
		err := backend.Upsert(ctx, database.UpsertConnectionLogParams{
			ID:               uuid.New(),
			Time:             connectTime,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceID:      ws.ID,
			WorkspaceName:    ws.Name,
			AgentName:        "main",
			Type:             database.ConnectionTypeSsh,
			ConnectionID:     uuid.NullUUID{UUID: connID, Valid: true},
			ConnectionStatus: database.ConnectionStatusConnected,
			IP:               testIP(),
		})
		require.NoError(t, err)

		err = backend.Upsert(ctx, database.UpsertConnectionLogParams{
			ID:               uuid.New(),
			Time:             disconnectTime,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceID:      ws.ID,
			WorkspaceName:    ws.Name,
			AgentName:        "main",
			Type:             database.ConnectionTypeSsh,
			ConnectionID:     uuid.NullUUID{UUID: connID, Valid: true},
			ConnectionStatus: database.ConnectionStatusDisconnected,
			Code:             sql.NullInt32{Int32: 0, Valid: true},
			DisconnectReason: sql.NullString{String: "done", Valid: true},
			IP:               testIP(),
		})
		require.NoError(t, err)

		// Close drains channel and flushes — dedup keeps disconnect.
		err = backend.Close()
		require.NoError(t, err)

		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
			LimitOpt: 10,
		})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.True(t, rows[0].ConnectionLog.DisconnectTime.Valid)
		require.Equal(t, "done", rows[0].ConnectionLog.DisconnectReason.String)
	})

	t.Run("MultipleIndependentConnections", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		clock := quartz.NewMock(t)

		ws := createWorkspace(t, db)

		//nolint:gocritic // Test needs system context for the batcher.
		backend := connectionlog.NewDBBatcher(
			dbauthz.AsConnectionLogger(ctx), db, log,
			connectionlog.WithClock(clock),
			connectionlog.WithBatchSize(100),
		)

		now := dbtime.Now()
		for i := 0; i < 5; i++ {
			err := backend.Upsert(ctx, database.UpsertConnectionLogParams{
				ID:               uuid.New(),
				Time:             now,
				OrganizationID:   ws.OrganizationID,
				WorkspaceOwnerID: ws.OwnerID,
				WorkspaceID:      ws.ID,
				WorkspaceName:    ws.Name,
				AgentName:        "main",
				Type:             database.ConnectionTypeSsh,
				ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
				ConnectionStatus: database.ConnectionStatusConnected,
				IP:               testIP(),
			})
			require.NoError(t, err)
		}

		err := backend.Close()
		require.NoError(t, err)

		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
			LimitOpt: 10,
		})
		require.NoError(t, err)
		require.Len(t, rows, 5)
	})

	t.Run("NullConnectionIDWebEvents", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		clock := quartz.NewMock(t)

		ws := createWorkspace(t, db)

		//nolint:gocritic // Test needs system context for the batcher.
		backend := connectionlog.NewDBBatcher(
			dbauthz.AsConnectionLogger(ctx), db, log,
			connectionlog.WithClock(clock),
			connectionlog.WithBatchSize(100),
		)

		now := dbtime.Now()
		for i := 0; i < 2; i++ {
			err := backend.Upsert(ctx, database.UpsertConnectionLogParams{
				ID:               uuid.New(),
				Time:             now,
				OrganizationID:   ws.OrganizationID,
				WorkspaceOwnerID: ws.OwnerID,
				WorkspaceID:      ws.ID,
				WorkspaceName:    ws.Name,
				AgentName:        "main",
				Type:             database.ConnectionTypeWorkspaceApp,
				ConnectionID:     uuid.NullUUID{},
				ConnectionStatus: database.ConnectionStatusConnected,
				IP:               testIP(),
			})
			require.NoError(t, err)
		}

		err := backend.Close()
		require.NoError(t, err)

		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
			LimitOpt: 10,
		})
		require.NoError(t, err)
		require.Len(t, rows, 2, "null connection_id events should not be deduplicated")
	})

	t.Run("CloseFlushesToDB", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		clock := quartz.NewMock(t)

		ws := createWorkspace(t, db)

		//nolint:gocritic // Test needs system context for the batcher.
		backend := connectionlog.NewDBBatcher(
			dbauthz.AsConnectionLogger(ctx), db, log,
			connectionlog.WithClock(clock),
			connectionlog.WithBatchSize(100),
		)

		err := backend.Upsert(ctx, database.UpsertConnectionLogParams{
			ID:               uuid.New(),
			Time:             dbtime.Now(),
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceID:      ws.ID,
			WorkspaceName:    ws.Name,
			AgentName:        "main",
			Type:             database.ConnectionTypeSsh,
			ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
			ConnectionStatus: database.ConnectionStatusConnected,
			IP:               testIP(),
		})
		require.NoError(t, err)

		// Close without advancing clock — final flush should write.
		err = backend.Close()
		require.NoError(t, err)

		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
			LimitOpt: 10,
		})
		require.NoError(t, err)
		require.Len(t, rows, 1)
	})
}
