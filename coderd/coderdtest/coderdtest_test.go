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
	client := coderdtest.New(t)
	user := coderdtest.CreateInitialUser(t, client)
	closer := coderdtest.NewProvisionerDaemon(t, client)
	project := coderdtest.CreateProject(t, client, user.Organization)
	version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, nil)
	coderdtest.AwaitProjectVersionImported(t, client, user.Organization, project.Name, version.Name)
	workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
	history, err := client.CreateWorkspaceHistory(context.Background(), "me", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
		ProjectVersionID: version.ID,
		Transition:       database.WorkspaceTransitionStart,
	})
	require.NoError(t, err)
	coderdtest.AwaitWorkspaceHistoryProvisioned(t, client, "me", workspace.Name, history.Name)
	closer.Close()
}
