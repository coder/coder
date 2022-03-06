package coderdtest_test

import (
	"context"
	"testing"

	"go.uber.org/goleak"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
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
	job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitProjectImportJob(t, client, user.OrganizationID, job.ID)
	project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
	workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
	history, err := client.CreateWorkspaceBuild(context.Background(), "me", workspace.Name, coderd.CreateWorkspaceBuildRequest{
		ProjectVersionID: project.ActiveVersionID,
		Transition:       database.WorkspaceTransitionStart,
	})
	require.NoError(t, err)
	coderdtest.AwaitWorkspaceProvisionJob(t, client, user.OrganizationID, history.ProvisionJobID)
	closer.Close()
}
