package database_test

import (
	"context"
	"database/sql"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

func TestCloseOpenAgentConnectionLogsForWorkspace(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := context.Background()

	u := dbgen.User(t, db, database.User{})
	o := dbgen.Organization(t, db, database.Organization{})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: o.ID,
		CreatedBy:      u.ID,
	})

	ws1 := dbgen.Workspace(t, db, database.WorkspaceTable{
		ID:               uuid.New(),
		OwnerID:          u.ID,
		OrganizationID:   o.ID,
		AutomaticUpdates: database.AutomaticUpdatesNever,
		TemplateID:       tpl.ID,
	})
	ws2 := dbgen.Workspace(t, db, database.WorkspaceTable{
		ID:               uuid.New(),
		OwnerID:          u.ID,
		OrganizationID:   o.ID,
		AutomaticUpdates: database.AutomaticUpdatesNever,
		TemplateID:       tpl.ID,
	})

	ip := pqtype.Inet{
		IPNet: net.IPNet{
			IP:   net.IPv4(127, 0, 0, 1),
			Mask: net.IPv4Mask(255, 255, 255, 255),
		},
		Valid: true,
	}

	// Simulate agent clock skew by using a connect time in the future.
	connectTime := dbtime.Now().Add(time.Hour)

	sshLog1, err := db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             connectTime,
		OrganizationID:   ws1.OrganizationID,
		WorkspaceOwnerID: ws1.OwnerID,
		WorkspaceID:      ws1.ID,
		WorkspaceName:    ws1.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSsh,
		Ip:               ip,
		ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})
	require.NoError(t, err)

	appLog, err := db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             dbtime.Now(),
		OrganizationID:   ws1.OrganizationID,
		WorkspaceOwnerID: ws1.OwnerID,
		WorkspaceID:      ws1.ID,
		WorkspaceName:    ws1.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeWorkspaceApp,
		Ip:               ip,
		UserAgent:        sql.NullString{String: "test", Valid: true},
		UserID:           uuid.NullUUID{UUID: ws1.OwnerID, Valid: true},
		SlugOrPort:       sql.NullString{String: "app", Valid: true},
		Code:             sql.NullInt32{Int32: 200, Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})
	require.NoError(t, err)

	sshLog2, err := db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             dbtime.Now(),
		OrganizationID:   ws2.OrganizationID,
		WorkspaceOwnerID: ws2.OwnerID,
		WorkspaceID:      ws2.ID,
		WorkspaceName:    ws2.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSsh,
		Ip:               ip,
		ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})
	require.NoError(t, err)

	rowsClosed, err := db.CloseOpenAgentConnectionLogsForWorkspace(ctx, database.CloseOpenAgentConnectionLogsForWorkspaceParams{
		WorkspaceID: ws1.ID,
		ClosedAt:    dbtime.Now(),
		Reason:      "workspace stopped",
		Types: []database.ConnectionType{
			database.ConnectionTypeSsh,
			database.ConnectionTypeVscode,
			database.ConnectionTypeJetbrains,
			database.ConnectionTypeReconnectingPty,
		},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rowsClosed)

	ws1Rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{WorkspaceID: ws1.ID})
	require.NoError(t, err)
	require.Len(t, ws1Rows, 2)

	for _, row := range ws1Rows {
		switch row.ConnectionLog.ID {
		case sshLog1.ID:
			updated := row.ConnectionLog
			require.True(t, updated.DisconnectTime.Valid)
			require.True(t, updated.DisconnectReason.Valid)
			require.Equal(t, "workspace stopped", updated.DisconnectReason.String)
			require.False(t, updated.DisconnectTime.Time.Before(updated.ConnectTime), "disconnect_time should never be before connect_time")
		case appLog.ID:
			notClosed := row.ConnectionLog
			require.False(t, notClosed.DisconnectTime.Valid)
			require.False(t, notClosed.DisconnectReason.Valid)
		default:
			t.Fatalf("unexpected connection log id: %s", row.ConnectionLog.ID)
		}
	}

	ws2Rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{WorkspaceID: ws2.ID})
	require.NoError(t, err)
	require.Len(t, ws2Rows, 1)
	require.Equal(t, sshLog2.ID, ws2Rows[0].ConnectionLog.ID)
	require.False(t, ws2Rows[0].ConnectionLog.DisconnectTime.Valid)
}
