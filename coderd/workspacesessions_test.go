package coderd_test

import (
	"context"
	"database/sql"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"

	"github.com/sqlc-dev/pqtype"
)

func TestWorkspaceSessions_EmptyResponse(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database: db,
		Pubsub:   ps,
	})

	user := coderdtest.CreateFirstUser(t, client)

	r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent().Do()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	resp, err := client.WorkspaceSessions(ctx, r.Workspace.ID)
	require.NoError(t, err)
	require.Empty(t, resp.Sessions)
	require.Equal(t, int64(0), resp.Count)
}

func TestWorkspaceSessions_WithHistoricSessions(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database: db,
		Pubsub:   ps,
	})

	user := coderdtest.CreateFirstUser(t, client)

	r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent().Do()

	now := dbtime.Now()

	// Insert two connected SSH connections from the same IP.
	connID1 := uuid.New()
	_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-30 * time.Minute),
		OrganizationID:   r.Workspace.OrganizationID,
		WorkspaceOwnerID: r.Workspace.OwnerID,
		WorkspaceID:      r.Workspace.ID,
		WorkspaceName:    r.Workspace.Name,
		AgentName:        r.Agents[0].Name,
		Type:             database.ConnectionTypeSsh,
		ConnectionID:     uuid.NullUUID{UUID: connID1, Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})

	connID2 := uuid.New()
	_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-25 * time.Minute),
		OrganizationID:   r.Workspace.OrganizationID,
		WorkspaceOwnerID: r.Workspace.OwnerID,
		WorkspaceID:      r.Workspace.ID,
		WorkspaceName:    r.Workspace.Name,
		AgentName:        r.Agents[0].Name,
		Type:             database.ConnectionTypeSsh,
		ConnectionID:     uuid.NullUUID{UUID: connID2, Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})

	// Close the connections and create sessions atomically.
	closedAt := now.Add(-5 * time.Minute)
	_, err := db.CloseConnectionLogsAndCreateSessions(context.Background(), database.CloseConnectionLogsAndCreateSessionsParams{
		ClosedAt:    sql.NullTime{Time: closedAt, Valid: true},
		Reason:      sql.NullString{String: "workspace stopped", Valid: true},
		WorkspaceID: r.Workspace.ID,
		Types:       []database.ConnectionType{database.ConnectionTypeSsh},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	resp, err := client.WorkspaceSessions(ctx, r.Workspace.ID)
	require.NoError(t, err)
	// CloseConnectionLogsAndCreateSessions groups by IP, so both
	// connections from 127.0.0.1 should be in a single session.
	require.Equal(t, int64(1), resp.Count)
	require.Len(t, resp.Sessions, 1)
	require.NotNil(t, resp.Sessions[0].IP)
	require.Equal(t, "127.0.0.1", resp.Sessions[0].IP.String())
	require.Equal(t, codersdk.ConnectionStatusCleanDisconnected, resp.Sessions[0].Status)
	require.Len(t, resp.Sessions[0].Connections, 2)
}

func TestWorkspaceAgentConnections_LiveSessionGrouping(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database: db,
		Pubsub:   ps,
	})

	user := coderdtest.CreateFirstUser(t, client)

	r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent().Do()

	now := dbtime.Now()

	// Two ongoing SSH connections from the same IP (127.0.0.1, the
	// default in dbgen).
	_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-2 * time.Minute),
		OrganizationID:   r.Workspace.OrganizationID,
		WorkspaceOwnerID: r.Workspace.OwnerID,
		WorkspaceID:      r.Workspace.ID,
		WorkspaceName:    r.Workspace.Name,
		AgentName:        r.Agents[0].Name,
		Type:             database.ConnectionTypeSsh,
		ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})

	_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-1 * time.Minute),
		OrganizationID:   r.Workspace.OrganizationID,
		WorkspaceOwnerID: r.Workspace.OwnerID,
		WorkspaceID:      r.Workspace.ID,
		WorkspaceName:    r.Workspace.Name,
		AgentName:        r.Agents[0].Name,
		Type:             database.ConnectionTypeSsh,
		ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})

	// One ongoing SSH connection from a different IP (10.0.0.1).
	_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-1 * time.Minute),
		OrganizationID:   r.Workspace.OrganizationID,
		WorkspaceOwnerID: r.Workspace.OwnerID,
		WorkspaceID:      r.Workspace.ID,
		WorkspaceName:    r.Workspace.Name,
		AgentName:        r.Agents[0].Name,
		Type:             database.ConnectionTypeSsh,
		ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
		Ip: pqtype.Inet{
			IPNet: net.IPNet{
				IP:   net.IPv4(10, 0, 0, 1),
				Mask: net.IPv4Mask(255, 255, 255, 255),
			},
			Valid: true,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	workspace, err := client.Workspace(ctx, r.Workspace.ID)
	require.NoError(t, err)
	require.NotEmpty(t, workspace.LatestBuild.Resources)
	require.NotEmpty(t, workspace.LatestBuild.Resources[0].Agents)

	agent := workspace.LatestBuild.Resources[0].Agents[0]
	require.Len(t, agent.Sessions, 2)

	// Find which session is which by IP.
	var session127, session10 codersdk.WorkspaceSession
	for _, s := range agent.Sessions {
		require.NotNil(t, s.IP)
		switch s.IP.String() {
		case "127.0.0.1":
			session127 = s
		case "10.0.0.1":
			session10 = s
		default:
			t.Fatalf("unexpected session IP: %s", s.IP.String())
		}
	}

	// The 127.0.0.1 session should have 2 connections.
	require.Len(t, session127.Connections, 2)
	require.Equal(t, codersdk.ConnectionStatusOngoing, session127.Status)

	// The 10.0.0.1 session should have 1 connection.
	require.Len(t, session10.Connections, 1)
	require.Equal(t, codersdk.ConnectionStatusOngoing, session10.Status)
}
