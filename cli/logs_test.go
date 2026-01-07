package cli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/google/uuid"

	"github.com/stretchr/testify/require"
)

func TestLogsCmd(t *testing.T) {
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	agentID := uuid.New()
	ws := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:        memberUser.ID,
		OrganizationID: owner.OrganizationID,
	}).WithAgent(func(a []*proto.Agent) []*proto.Agent {
		a[0].Id = agentID.String()
		return a
	}).Do()
	setupCtx := testutil.Context(t, testutil.WaitShort)
	_, err := db.InsertProvisionerJobLogs(setupCtx, database.InsertProvisionerJobLogsParams{
		JobID:     ws.Build.JobID,
		CreatedAt: []time.Time{ws.Build.UpdatedAt},
		Stage:     []string{"Test"},
		Level:     []database.LogLevel{database.LogLevelInfo},
		Source:    []database.LogSource{database.LogSourceProvisioner},
		Output:    []string{"test provisioner log"},
	})
	require.NoError(t, err, "insert provisioner job logs")
	_, err = db.InsertWorkspaceAgentLogs(setupCtx, database.InsertWorkspaceAgentLogsParams{
		AgentID:     agentID,
		CreatedAt:   ws.Build.UpdatedAt,
		Level:       []database.LogLevel{database.LogLevelInfo},
		LogSourceID: uuid.New(),
		Output:      []string{"test agent log"},
	})
	require.NoError(t, err, "insert workspace agent logs")

	t.Run("workspace not found", func(t *testing.T) {
		t.Parallel()

		inv, root := clitest.New(t, "logs", "doesnotexist")
		clitest.SetupConfig(t, memberClient, root)
		ctx := testutil.Context(t, testutil.WaitShort)
		var stdout strings.Builder
		inv.Stdout = &stdout
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, stdout.String(), "failed to get workspace")
	})

	t.Run("latest build logs", func(t *testing.T) {
		t.Parallel()

		inv, root := clitest.New(t, "logs", ws.Workspace.Name)
		clitest.SetupConfig(t, memberClient, root)
		ctx := testutil.Context(t, testutil.WaitShort)
		var stdout strings.Builder
		inv.Stdout = &stdout
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, stdout.String(), "test provisioner log")
		require.Contains(t, stdout.String(), "test agent log")
	})
}
