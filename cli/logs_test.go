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
	"github.com/google/uuid"

	"github.com/stretchr/testify/require"
)

func TestLogsCmd(t *testing.T) {
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	testWorkspace := func(t testing.TB, db database.Store, ownerID, orgID uuid.UUID) dbfake.WorkspaceResponse {
		wb := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        memberUser.ID,
			OrganizationID: owner.OrganizationID,
		}).WithAgent().Do()
		_ = dbgen.ProvisionerJobLog(t, db, database.ProvisionerJobLog{
			JobID:  wb.Build.JobID,
			Output: "test provisioner log for build " + wb.Build.ID.String(),
		})
		for _, agt := range wb.Agents {
			_ = dbgen.WorkspaceAgentLog(t, db, database.WorkspaceAgentLog{
				AgentID: agt.ID,
				Output:  "test agent log for agent " + agt.ID.String(),
			})
		}
		return wb
	}

	assertLogOutput := func(t testing.TB, wb dbfake.WorkspaceResponse, output string) {
		t.Helper()
		require.Contains(t, output, "test provisioner log for build "+wb.Build.ID.String())
		for _, agt := range wb.Agents {
			require.Contains(t, output, "test agent log for agent "+agt.ID.String())
		}
	}

	assertAntagonist := func(t testing.TB, wb dbfake.WorkspaceResponse, output string) {
		t.Helper()
		require.NotContains(t, output, "test provisioner log for build "+wb.Build.ID.String())
		for _, agt := range wb.Agents {
			require.NotContains(t, output, "test agent log for agent "+agt.ID.String())
		}
	}

	wb1 := testWorkspace(t, db, memberUser.ID, owner.OrganizationID)
	wb2 := testWorkspace(t, db, owner.UserID, owner.OrganizationID)

	t.Run("workspace not found", func(t *testing.T) {
		t.Parallel()

		inv, root := clitest.New(t, "logs", "doesnotexist")
		clitest.SetupConfig(t, memberClient, root)
		ctx := testutil.Context(t, testutil.WaitShort)
		var stdout strings.Builder
		inv.Stdout = &stdout
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "Resource not found or you do not have access to this resource")
	})

	// Note: not testing with --follow as it is inherently racy.
	t.Run("current build", func(t *testing.T) {
		t.Parallel()

		inv, root := clitest.New(t, "logs", wb1.Workspace.Name)
		clitest.SetupConfig(t, memberClient, root)
		ctx := testutil.Context(t, testutil.WaitShort)
		var stdout strings.Builder
		inv.Stdout = &stdout
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err, "failed to fetch logs for current build")
		assertLogOutput(t, wb1, stdout.String())
		assertAntagonist(t, wb2, stdout.String())
	})

	t.Run("specific build", func(t *testing.T) {
		t.Parallel()

		inv, root := clitest.New(t, "logs", wb1.Workspace.Name, "-n", fmt.Sprintf("%d", wb1.Build.BuildNumber))
		clitest.SetupConfig(t, memberClient, root)
		ctx := testutil.Context(t, testutil.WaitShort)
		var stdout strings.Builder
		inv.Stdout = &stdout
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err, "failed to fetch logs for specific build")
		assertLogOutput(t, wb1, stdout.String())
		assertAntagonist(t, wb2, stdout.String())
	})

	t.Run("build out of range", func(t *testing.T) {
		t.Parallel()

		inv, root := clitest.New(t, "logs", wb1.Workspace.Name, "-n", "-9999")
		clitest.SetupConfig(t, memberClient, root)
		ctx := testutil.Context(t, testutil.WaitShort)
		var stdout strings.Builder
		inv.Stdout = &stdout
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "invalid build number offset")
	})
}
