package coderdtest_test

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/coder/coder/coderd/coderdtest"
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
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)
	closer.Close()
}
