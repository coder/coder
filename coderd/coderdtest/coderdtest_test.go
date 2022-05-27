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
	client, coderAPI := coderdtest.NewWithAPI(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	closer := coderdtest.NewProvisionerDaemon(t, coderAPI)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)
	_, _ = coderdtest.NewGoogleInstanceIdentity(t, "example", false)
	_, _ = coderdtest.NewAWSInstanceIdentity(t, "an-instance")
	closer.Close()
}
