package database_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

func TestGetOngoingAgentConnectionsLast24h(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, _ := dbtestutil.NewDB(t)

	org := dbfake.Organization(t, db).Do()
	user := dbgen.User(t, db, database.User{})
	tpl := dbgen.Template(t, db, database.Template{OrganizationID: org.Org.ID, CreatedBy: user.ID})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.Org.ID,
		OwnerID:        user.ID,
		TemplateID:     tpl.ID,
		Name:           "ws",
	})

	now := dbtime.Now()
	since := now.Add(-24 * time.Hour)

	const (
		agent1 = "agent1"
		agent2 = "agent2"
	)

	// Insert a disconnected log that should be excluded.
	disconnectedConnID := uuid.New()
	disconnected := dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-30 * time.Minute),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        agent1,
		Type:             database.ConnectionTypeSsh,
		ConnectionStatus: database.ConnectionStatusConnected,
		ConnectionID:     uuid.NullUUID{UUID: disconnectedConnID, Valid: true},
	})
	_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-20 * time.Minute),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		AgentName:        disconnected.AgentName,
		ConnectionStatus: database.ConnectionStatusDisconnected,
		ConnectionID:     disconnected.ConnectionID,
		DisconnectReason: sql.NullString{String: "closed", Valid: true},
	})

	// Insert an old log that should be excluded by the 24h window.
	_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-25 * time.Hour),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        agent1,
		Type:             database.ConnectionTypeSsh,
		ConnectionStatus: database.ConnectionStatusConnected,
		ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
	})

	// Insert a web log that should be excluded by the types filter.
	_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-10 * time.Minute),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        agent1,
		Type:             database.ConnectionTypeWorkspaceApp,
		ConnectionStatus: database.ConnectionStatusConnected,
		ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
	})

	// Insert 55 active logs for agent1 (should be capped to 50).
	for i := 0; i < 55; i++ {
		_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			Time:             now.Add(-time.Duration(i) * time.Minute),
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceID:      ws.ID,
			WorkspaceName:    ws.Name,
			AgentName:        agent1,
			Type:             database.ConnectionTypeVscode,
			ConnectionStatus: database.ConnectionStatusConnected,
			ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
		})
	}

	// Insert one active log for agent2.
	agent2Log := dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-5 * time.Minute),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        agent2,
		Type:             database.ConnectionTypeJetbrains,
		ConnectionStatus: database.ConnectionStatusConnected,
		ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
	})

	logs, err := db.GetOngoingAgentConnectionsLast24h(ctx, database.GetOngoingAgentConnectionsLast24hParams{
		WorkspaceIds:  []uuid.UUID{ws.ID},
		AgentNames:    []string{agent1, agent2},
		Types:         []database.ConnectionType{database.ConnectionTypeSsh, database.ConnectionTypeVscode, database.ConnectionTypeJetbrains, database.ConnectionTypeReconnectingPty},
		Since:         since,
		PerAgentLimit: 50,
	})
	require.NoError(t, err)

	byAgent := map[string][]database.GetOngoingAgentConnectionsLast24hRow{}
	for _, l := range logs {
		byAgent[l.AgentName] = append(byAgent[l.AgentName], l)
	}

	// Agent1 should be capped at 50 and contain only active logs within the window.
	require.Len(t, byAgent[agent1], 50)
	for i, l := range byAgent[agent1] {
		require.False(t, l.DisconnectTime.Valid, "expected log to be ongoing")
		require.True(t, l.ConnectTime.After(since) || l.ConnectTime.Equal(since), "expected log to be within window")
		if i > 0 {
			require.True(t, byAgent[agent1][i-1].ConnectTime.After(l.ConnectTime) || byAgent[agent1][i-1].ConnectTime.Equal(l.ConnectTime), "expected logs to be ordered by connect_time desc")
		}
	}

	// Agent2 should include its single active log.
	require.Equal(t, []uuid.UUID{agent2Log.ID}, []uuid.UUID{byAgent[agent2][0].ID})
}
