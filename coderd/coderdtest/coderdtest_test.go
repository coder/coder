package coderdtest_test

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestNew(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
	_, _ = coderdtest.NewGoogleInstanceIdentity(t, "example", false)
	_, _ = coderdtest.NewAWSInstanceIdentity(t, "an-instance")
}
