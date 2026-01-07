package cli_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/testutil"

	"github.com/stretchr/testify/require"
)

func TestLogsCmd(t *testing.T) {
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	wb := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:        memberUser.ID,
		OrganizationID: owner.OrganizationID,
	}).WithAgent().Do()
	jobLog := dbgen.ProvisionerJobLog(t, db, database.ProvisionerJobLog{
		JobID:  wb.Build.JobID,
		Output: "test provisioner log",
	})
	var agentlogs []database.WorkspaceAgentLog
	for _, agt := range wb.Agents {
		agentlog := dbgen.WorkspaceAgentLog(t, db, database.WorkspaceAgentLog{
			AgentID: agt.ID,
			Output:  "test agent log for " + agt.ID.String(),
		})
		agentlogs = append(agentlogs, agentlog)
	}

	t.Run("workspace not found", func(t *testing.T) {
		t.Parallel()

		inv, root := clitest.New(t, "logs", "doesnotexist")
		clitest.SetupConfig(t, memberClient, root)
		ctx := testutil.Context(t, testutil.WaitShort)
		var stdout strings.Builder
		inv.Stdout = &stdout
		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
	})

	// Note: not testing with --follow as it is inherently racy.
	t.Run("current build", func(t *testing.T) {
		t.Parallel()

		inv, root := clitest.New(t, "logs", wb.Workspace.Name)
		clitest.SetupConfig(t, memberClient, root)
		ctx := testutil.Context(t, testutil.WaitShort)
		var stdout strings.Builder
		inv.Stdout = &stdout
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, stdout.String(), jobLog.Output)
		for _, log := range agentlogs {
			require.Contains(t, stdout.String(), log.Output)
		}
	})

	t.Run("specific build", func(t *testing.T) {
		t.Parallel()

		inv, root := clitest.New(t, "logs", wb.Workspace.Name, "-n", fmt.Sprintf("%d", wb.Build.BuildNumber))
		clitest.SetupConfig(t, memberClient, root)
		ctx := testutil.Context(t, testutil.WaitShort)
		var stdout strings.Builder
		inv.Stdout = &stdout
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, stdout.String(), jobLog.Output)
		for _, log := range agentlogs {
			require.Contains(t, stdout.String(), log.Output)
		}
	})

	t.Run("build out of range", func(t *testing.T) {
		t.Parallel()

		inv, root := clitest.New(t, "logs", wb.Workspace.Name, "-n", "-9999")
		clitest.SetupConfig(t, memberClient, root)
		ctx := testutil.Context(t, testutil.WaitShort)
		var stdout strings.Builder
		inv.Stdout = &stdout
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "invalid build number offset")
	})
}
