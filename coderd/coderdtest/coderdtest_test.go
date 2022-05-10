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
	api := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, api.Client)
	closer := coderdtest.NewProvisionerDaemon(t, api.Client)
	version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
	template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, api.Client, workspace.LatestBuild.ID)
	coderdtest.AwaitWorkspaceAgents(t, api.Client, workspace.LatestBuild.ID)
	_, _ = coderdtest.NewGoogleInstanceIdentity(t, "example", false)
	_, _ = coderdtest.NewAWSInstanceIdentity(t, "an-instance")
	closer.Close()
}
