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
	user := coderdtest.CreateInitialUser(t, client)
	closer := coderdtest.NewProvisionerDaemon(t, client)
	job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
	coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
	project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
	workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
	history, err := client.CreateWorkspaceHistory(context.Background(), "me", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
		ProjectVersionID: project.ActiveVersionID,
		Transition:       database.WorkspaceTransitionStart,
	})
	require.NoError(t, err)
	coderdtest.AwaitWorkspaceProvisionJob(t, client, user.Organization, history.ProvisionJobID)
	closer.Close()
}
