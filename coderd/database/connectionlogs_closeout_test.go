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
			// NULL-IP system connection overlaps with the SSH
			// session, so it gets attached to that session.
			require.True(t, cl.SessionID.Valid,
				"NULL-IP system log overlapping with SSH session should be linked to a session")
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

// Test: connections with different IPs but same hostname get grouped
// into one session.
func TestCloseConnectionLogsAndCreateSessions_GroupsByHostname(t *testing.T) {
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

	now := dbtime.Now()
	hostname := sql.NullString{String: "my-laptop", Valid: true}

	// Create 3 SSH connections with different IPs but same hostname,
	// overlapping in time.
	var logIDs []uuid.UUID
	for i := 0; i < 3; i++ {
		ip := pqtype.Inet{
			IPNet: net.IPNet{
				IP:   net.IPv4(10, 0, 0, byte(i+1)),
				Mask: net.IPv4Mask(255, 255, 255, 255),
			},
			Valid: true,
		}
		log, err := db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
			ID:               uuid.New(),
			Time:             now.Add(time.Duration(-30+i*5) * time.Minute),
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceID:      ws.ID,
			WorkspaceName:    ws.Name,
			AgentName:        "agent",
			Type:             database.ConnectionTypeSsh,
			Ip:               ip,
			ClientHostname:   hostname,
			ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
			ConnectionStatus: database.ConnectionStatusConnected,
		})
		require.NoError(t, err)
		logIDs = append(logIDs, log.ID)
	}

	closedAt := now
	_, err := db.CloseConnectionLogsAndCreateSessions(ctx, database.CloseConnectionLogsAndCreateSessionsParams{
		ClosedAt:    sql.NullTime{Time: closedAt, Valid: true},
		Reason:      sql.NullString{String: "workspace stopped", Valid: true},
		WorkspaceID: ws.ID,
		Types: []database.ConnectionType{
			database.ConnectionTypeSsh,
		},
	})
	require.NoError(t, err)

	rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
		WorkspaceID: ws.ID,
	})
	require.NoError(t, err)
	require.Len(t, rows, 3)

	// All 3 connections should have the same session_id.
	var sessionID uuid.UUID
	for i, row := range rows {
		cl := row.ConnectionLog
		require.True(t, cl.SessionID.Valid,
			"connection %d should have a session", i)
		if i == 0 {
			sessionID = cl.SessionID.UUID
		} else {
			require.Equal(t, sessionID, cl.SessionID.UUID,
				"all connections with same hostname should share one session")
		}
	}
}

// Test: a long-running system connection gets attached to the first
// overlapping primary session, not the second.
func TestCloseConnectionLogsAndCreateSessions_SystemAttachesToFirstSession(t *testing.T) {
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
			IP:   net.IPv4(10, 0, 0, 1),
			Mask: net.IPv4Mask(255, 255, 255, 255),
		},
		Valid: true,
	}
	now := dbtime.Now()

	// System connection spanning the full workspace lifetime.
	sysLog, err := db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-3 * time.Hour),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSystem,
		Ip:               ip,
		ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})
	require.NoError(t, err)

	// SSH session 1: -3h to -2h.
	ssh1ConnID := uuid.New()
	_, err = db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-3 * time.Hour),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSsh,
		Ip:               ip,
		ConnectionID:     uuid.NullUUID{UUID: ssh1ConnID, Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})
	require.NoError(t, err)
	ssh1Disc, err := db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-2 * time.Hour),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSsh,
		Ip:               ip,
		ConnectionID:     uuid.NullUUID{UUID: ssh1ConnID, Valid: true},
		ConnectionStatus: database.ConnectionStatusDisconnected,
	})
	require.NoError(t, err)
	_ = ssh1Disc

	// SSH session 2: -30min to now (>30min gap from session 1).
	_, err = db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-30 * time.Minute),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSsh,
		Ip:               ip,
		ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})
	require.NoError(t, err)

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

	rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
		WorkspaceID: ws.ID,
	})
	require.NoError(t, err)

	// Find the system connection and its assigned session.
	var sysSessionID uuid.UUID
	// Collect all session IDs from SSH connections to verify 2
	// distinct sessions were created.
	sshSessionIDs := make(map[uuid.UUID]bool)
	for _, row := range rows {
		cl := row.ConnectionLog
		if cl.ID == sysLog.ID {
			require.True(t, cl.SessionID.Valid,
				"system connection should have a session")
			sysSessionID = cl.SessionID.UUID
		}
		if cl.Type == database.ConnectionTypeSsh && cl.SessionID.Valid {
			sshSessionIDs[cl.SessionID.UUID] = true
		}
	}

	// Two distinct SSH sessions should exist (>30min gap).
	require.Len(t, sshSessionIDs, 2, "should have 2 distinct SSH sessions")

	// System connection should be attached to the first (earliest)
	// session.
	require.True(t, sshSessionIDs[sysSessionID],
		"system connection should be attached to one of the SSH sessions")
}

// Test: an orphaned system connection (no overlapping primary sessions)
// with an IP gets its own session.
func TestCloseConnectionLogsAndCreateSessions_OrphanSystemGetsOwnSession(t *testing.T) {
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
			IP:   net.IPv4(10, 0, 0, 1),
			Mask: net.IPv4Mask(255, 255, 255, 255),
		},
		Valid: true,
	}
	now := dbtime.Now()

	// System connection with an IP but no overlapping primary
	// connections.
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
		ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})
	require.NoError(t, err)

	closedAt := now
	_, err = db.CloseConnectionLogsAndCreateSessions(ctx, database.CloseConnectionLogsAndCreateSessionsParams{
		ClosedAt:    sql.NullTime{Time: closedAt, Valid: true},
		Reason:      sql.NullString{String: "workspace stopped", Valid: true},
		WorkspaceID: ws.ID,
		Types: []database.ConnectionType{
			database.ConnectionTypeSystem,
		},
	})
	require.NoError(t, err)

	rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
		WorkspaceID: ws.ID,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].ConnectionLog.SessionID.Valid,
		"orphaned system connection with IP should get its own session")
}

// Test: a system connection with NULL IP and no overlapping primary
// sessions gets no session (can't create a useful session without IP).
func TestCloseConnectionLogsAndCreateSessions_SystemNoIPNoSession(t *testing.T) {
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

	now := dbtime.Now()

	// System connection with NULL IP and no overlapping primary.
	_, err := db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-10 * time.Minute),
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

	closedAt := now
	_, err = db.CloseConnectionLogsAndCreateSessions(ctx, database.CloseConnectionLogsAndCreateSessionsParams{
		ClosedAt:    sql.NullTime{Time: closedAt, Valid: true},
		Reason:      sql.NullString{String: "workspace stopped", Valid: true},
		WorkspaceID: ws.ID,
		Types: []database.ConnectionType{
			database.ConnectionTypeSystem,
		},
	})
	require.NoError(t, err)

	rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
		WorkspaceID: ws.ID,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].ConnectionLog.DisconnectTime.Valid,
		"system connection should be closed")
	require.False(t, rows[0].ConnectionLog.SessionID.Valid,
		"NULL-IP system connection with no primary overlap should not get a session")
}

// Test: connections from the same hostname with a >30-minute gap
// create separate sessions.
func TestCloseConnectionLogsAndCreateSessions_SeparateSessionsForLargeGap(t *testing.T) {
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
			IP:   net.IPv4(10, 0, 0, 1),
			Mask: net.IPv4Mask(255, 255, 255, 255),
		},
		Valid: true,
	}
	now := dbtime.Now()

	// SSH connection 1: -3h to -2h.
	conn1ID := uuid.New()
	_, err := db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-3 * time.Hour),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSsh,
		Ip:               ip,
		ConnectionID:     uuid.NullUUID{UUID: conn1ID, Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})
	require.NoError(t, err)
	_, err = db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-2 * time.Hour),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSsh,
		Ip:               ip,
		ConnectionID:     uuid.NullUUID{UUID: conn1ID, Valid: true},
		ConnectionStatus: database.ConnectionStatusDisconnected,
	})
	require.NoError(t, err)

	// SSH connection 2: -30min to now (>30min gap from connection 1).
	_, err = db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now.Add(-30 * time.Minute),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        "agent",
		Type:             database.ConnectionTypeSsh,
		Ip:               ip,
		ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})
	require.NoError(t, err)

	closedAt := now
	_, err = db.CloseConnectionLogsAndCreateSessions(ctx, database.CloseConnectionLogsAndCreateSessionsParams{
		ClosedAt:    sql.NullTime{Time: closedAt, Valid: true},
		Reason:      sql.NullString{String: "workspace stopped", Valid: true},
		WorkspaceID: ws.ID,
		Types: []database.ConnectionType{
			database.ConnectionTypeSsh,
		},
	})
	require.NoError(t, err)

	rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
		WorkspaceID: ws.ID,
	})
	require.NoError(t, err)

	sessionIDs := make(map[uuid.UUID]bool)
	for _, row := range rows {
		cl := row.ConnectionLog
		if cl.SessionID.Valid {
			sessionIDs[cl.SessionID.UUID] = true
		}
	}
	require.Len(t, sessionIDs, 2,
		"connections with >30min gap should create 2 separate sessions")
}
