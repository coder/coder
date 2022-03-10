package coderdtest_test

import (
	"context"
	"testing"

	"go.uber.org/goleak"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestNew(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	closer := coderdtest.NewProvisionerDaemon(t, client)
	version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitProjectVersionJob(t, client, version.ID)
	project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
	build, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		ProjectVersionID: project.ActiveVersionID,
		Transition:       database.WorkspaceTransitionStart,
	})
	require.NoError(t, err)
	coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)
	coderdtest.AwaitWorkspaceAgents(t, client, build.ID)
	closer.Close()
}
