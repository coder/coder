package database_test

import (
	"context"
	"database/sql"
	"fmt"
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

// Regression test: CloseConnectionLogsAndCreateSessions must not fail
// when connection_logs have NULL IPs (e.g., disconnect-only tunnel
// events). NULL-IP logs should be closed but no session created for
// them.
func TestCloseConnectionLogsAndCreateSessions_NullIP(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := context.Background()

	u := dbgen.User(t, db, database.User{})
	o := dbgen.Organization(t, db, database.Organization{})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: o.ID,
		CreatedBy:      u.ID,
	})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:          u.ID,
		OrganizationID:   o.ID,
		AutomaticUpdates: database.AutomaticUpdatesNever,
		TemplateID:       tpl.ID,
	})

	validIP := pqtype.Inet{
		IPNet: net.IPNet{
			IP:   net.IPv4(10, 0, 0, 1),
			Mask: net.IPv4Mask(255, 255, 255, 255),
		},
		Valid: true,
	}
	now := dbtime.Now()

	// Connection with a valid IP.
	sshLog, err := db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-30 * time.Minute),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSsh,
		Ip:               validIP,
		ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})
	require.NoError(t, err)

	// Connection with a NULL IP — simulates a disconnect-only tunnel
	// event where the source node info is unavailable.
	nullIPLog, err := db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-25 * time.Minute),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSystem,
		Ip:               pqtype.Inet{Valid: false},
		ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})
	require.NoError(t, err)

	// This previously failed with: "pq: null value in column ip of
	// relation workspace_sessions violates not-null constraint".
	closedAt := now.Add(-5 * time.Minute)
	_, err = db.CloseConnectionLogsAndCreateSessions(ctx, database.CloseConnectionLogsAndCreateSessionsParams{
		ClosedAt:    sql.NullTime{Time: closedAt, Valid: true},
		Reason:      sql.NullString{String: "workspace stopped", Valid: true},
		WorkspaceID: ws.ID,
		Types: []database.ConnectionType{
			database.ConnectionTypeSsh,
			database.ConnectionTypeSystem,
		},
	})
	require.NoError(t, err)

	// Verify both logs were closed.
	rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
		WorkspaceID: ws.ID,
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)

	for _, row := range rows {
		cl := row.ConnectionLog
		require.True(t, cl.DisconnectTime.Valid,
			"connection log %s (type=%s) should be closed", cl.ID, cl.Type)

		switch cl.ID {
		case sshLog.ID:
			// Valid-IP log should have a session.
			require.True(t, cl.SessionID.Valid,
				"valid-IP log should be linked to a session")
		case nullIPLog.ID:
			// NULL-IP log should NOT have a session.
			require.False(t, cl.SessionID.Valid,
				"NULL-IP log should not be linked to a session")
		default:
			t.Fatalf("unexpected connection log id: %s", cl.ID)
		}
	}
}

// Regression test: CloseConnectionLogsAndCreateSessions must handle
// connections that are already disconnected but have no session_id
// (e.g., system/tunnel connections disconnected by dbsink). It must
// also avoid creating duplicate sessions when assignSessionForDisconnect
// has already created one for the same IP/time range.
func TestCloseConnectionLogsAndCreateSessions_AlreadyDisconnectedGetsSession(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := context.Background()

	u := dbgen.User(t, db, database.User{})
	o := dbgen.Organization(t, db, database.Organization{})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: o.ID,
		CreatedBy:      u.ID,
	})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
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
	now := dbtime.Now()

	// A system connection that was already disconnected (by dbsink)
	// but has no session_id — dbsink doesn't assign sessions.
	sysConnID := uuid.New()
	_, err := db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-10 * time.Minute),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSystem,
		Ip:               ip,
		ConnectionID:     uuid.NullUUID{UUID: sysConnID, Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})
	require.NoError(t, err)
	_, err = db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-5 * time.Minute),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSystem,
		Ip:               ip,
		ConnectionID:     uuid.NullUUID{UUID: sysConnID, Valid: true},
		ConnectionStatus: database.ConnectionStatusDisconnected,
	})
	require.NoError(t, err)

	// Run CloseConnectionLogsAndCreateSessions (workspace stop).
	closedAt := now
	_, err = db.CloseConnectionLogsAndCreateSessions(ctx, database.CloseConnectionLogsAndCreateSessionsParams{
		ClosedAt:    sql.NullTime{Time: closedAt, Valid: true},
		Reason:      sql.NullString{String: "workspace stopped", Valid: true},
		WorkspaceID: ws.ID,
		Types: []database.ConnectionType{
			database.ConnectionTypeSsh,
			database.ConnectionTypeSystem,
		},
	})
	require.NoError(t, err)

	// The system connection should now have a session_id.
	rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
		WorkspaceID: ws.ID,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].ConnectionLog.SessionID.Valid,
		"already-disconnected system connection should be assigned to a session")
}

// Regression test: when assignSessionForDisconnect has already
// created a session for an SSH connection,
// CloseConnectionLogsAndCreateSessions must reuse that session
// instead of creating a duplicate.
func TestCloseConnectionLogsAndCreateSessions_ReusesExistingSession(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := context.Background()

	u := dbgen.User(t, db, database.User{})
	o := dbgen.Organization(t, db, database.Organization{})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: o.ID,
		CreatedBy:      u.ID,
	})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
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
	now := dbtime.Now()

	// Simulate an SSH connection where assignSessionForDisconnect
	// already created a session but the connection log's session_id
	// was set (the normal successful path).
	sshConnID := uuid.New()
	_, err := db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-10 * time.Minute),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSsh,
		Ip:               ip,
		ConnectionID:     uuid.NullUUID{UUID: sshConnID, Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})
	require.NoError(t, err)
	sshLog, err := db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-5 * time.Minute),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSsh,
		Ip:               ip,
		ConnectionID:     uuid.NullUUID{UUID: sshConnID, Valid: true},
		ConnectionStatus: database.ConnectionStatusDisconnected,
	})
	require.NoError(t, err)

	// Create the session that assignSessionForDisconnect would have
	// created, and link the connection log to it.
	existingSessionIDRaw, err := db.FindOrCreateSessionForDisconnect(ctx, database.FindOrCreateSessionForDisconnectParams{
		WorkspaceID:    ws.ID.String(),
		Ip:             ip,
		ConnectTime:    sshLog.ConnectTime,
		DisconnectTime: sshLog.DisconnectTime.Time,
	})
	require.NoError(t, err)
	existingSessionID, err := uuid.Parse(fmt.Sprintf("%s", existingSessionIDRaw))
	require.NoError(t, err)
	err = db.UpdateConnectionLogSessionID(ctx, database.UpdateConnectionLogSessionIDParams{
		ID:        sshLog.ID,
		SessionID: uuid.NullUUID{UUID: existingSessionID, Valid: true},
	})
	require.NoError(t, err)

	// Also add a system connection (no session, already disconnected).
	sysConnID := uuid.New()
	_, err = db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-10 * time.Minute),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSystem,
		Ip:               ip,
		ConnectionID:     uuid.NullUUID{UUID: sysConnID, Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})
	require.NoError(t, err)
	_, err = db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-5 * time.Minute),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSystem,
		Ip:               ip,
		ConnectionID:     uuid.NullUUID{UUID: sysConnID, Valid: true},
		ConnectionStatus: database.ConnectionStatusDisconnected,
	})
	require.NoError(t, err)

	// Run CloseConnectionLogsAndCreateSessions.
	closedAt := now
	_, err = db.CloseConnectionLogsAndCreateSessions(ctx, database.CloseConnectionLogsAndCreateSessionsParams{
		ClosedAt:    sql.NullTime{Time: closedAt, Valid: true},
		Reason:      sql.NullString{String: "workspace stopped", Valid: true},
		WorkspaceID: ws.ID,
		Types: []database.ConnectionType{
			database.ConnectionTypeSsh,
			database.ConnectionTypeSystem,
		},
	})
	require.NoError(t, err)

	// Verify: the system connection should be assigned to the
	// EXISTING session (reused), not a new one.
	rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
		WorkspaceID: ws.ID,
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)

	for _, row := range rows {
		cl := row.ConnectionLog
		require.True(t, cl.SessionID.Valid,
			"connection log %s (type=%s) should have a session", cl.ID, cl.Type)
		require.Equal(t, existingSessionID, cl.SessionID.UUID,
			"connection log %s should reuse the existing session, not create a new one", cl.ID)
	}
}
