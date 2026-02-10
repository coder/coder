package coderd_test

import (
	"context"
	"database/sql"
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
)

func TestWorkspaceAgentConnections_FromConnectionLogs(t *testing.T) {
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

	// One active SSH connection should be returned.
	sshConnID := uuid.New()
	_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-1 * time.Minute),
		OrganizationID:   r.Workspace.OrganizationID,
		WorkspaceOwnerID: r.Workspace.OwnerID,
		WorkspaceID:      r.Workspace.ID,
		WorkspaceName:    r.Workspace.Name,
		AgentName:        r.Agents[0].Name,
		Type:             database.ConnectionTypeSsh,
		ConnectionID:     uuid.NullUUID{UUID: sshConnID, Valid: true},
		ConnectionStatus: database.ConnectionStatusConnected,
	})

	// A web-ish connection type should be ignored.
	// Use a time outside the 5-minute activity window so this
	// localhost web connection is treated as stale and filtered out.
	_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-10 * time.Minute),
		OrganizationID:   r.Workspace.OrganizationID,
		WorkspaceOwnerID: r.Workspace.OwnerID,
		WorkspaceID:      r.Workspace.ID,
		WorkspaceName:    r.Workspace.Name,
		AgentName:        r.Agents[0].Name,
		Type:             database.ConnectionTypeWorkspaceApp,
		ConnectionStatus: database.ConnectionStatusConnected,
		UserID:           uuid.NullUUID{UUID: user.UserID, Valid: true},
		UserAgent:        sql.NullString{String: "Mozilla/5.0", Valid: true},
		SlugOrPort:       sql.NullString{String: "code-server", Valid: true},
	})

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	workspace, err := client.Workspace(ctx, r.Workspace.ID)
	require.NoError(t, err)
	require.NotEmpty(t, workspace.LatestBuild.Resources)
	require.NotEmpty(t, workspace.LatestBuild.Resources[0].Agents)

	agent := workspace.LatestBuild.Resources[0].Agents[0]
	require.Equal(t, r.Agents[0].Name, agent.Name)
	require.Len(t, agent.Connections, 1)
	require.Equal(t, codersdk.ConnectionStatusOngoing, agent.Connections[0].Status)
	require.Equal(t, codersdk.ConnectionTypeSSH, agent.Connections[0].Type)
	require.NotNil(t, agent.Connections[0].IP)
	require.Equal(t, "127.0.0.1", agent.Connections[0].IP.String())

	apiAgent, err := client.WorkspaceAgent(ctx, agent.ID)
	require.NoError(t, err)
	require.Len(t, apiAgent.Connections, 1)
	require.Equal(t, codersdk.ConnectionTypeSSH, apiAgent.Connections[0].Type)
}
