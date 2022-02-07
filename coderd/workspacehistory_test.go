package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestPostWorkspaceHistoryByUser(t *testing.T) {
	t.Parallel()
	t.Run("NoProjectVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		_, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: uuid.New(),
			Transition:       database.WorkspaceTransitionStart,
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("ProjectVersionFailedImport", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, &echo.Responses{
			Provision: []*proto.Provision_Response{{}},
		})
		coderdtest.AwaitProjectVersionImported(t, client, user.Organization, project.Name, version.Name)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		_, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: version.ID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})

	t.Run("AlreadyActive", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		closeDaemon := coderdtest.NewProvisionerDaemon(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, nil)
		coderdtest.AwaitProjectVersionImported(t, client, user.Organization, project.Name, version.Name)
		// Close here so workspace history doesn't process!
		closeDaemon.Close()
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		_, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: version.ID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		_, err = client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: version.ID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("UpdatePriorAfterField", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, nil)
		coderdtest.AwaitProjectVersionImported(t, client, user.Organization, project.Name, version.Name)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		firstHistory, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: version.ID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceHistoryProvisioned(t, client, "me", workspace.Name, firstHistory.Name)
		secondHistory, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: version.ID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		require.Equal(t, firstHistory.ID.String(), secondHistory.BeforeID.String())

		firstHistory, err = client.WorkspaceHistory(context.Background(), "", workspace.Name, firstHistory.Name)
		require.NoError(t, err)
		require.Equal(t, secondHistory.ID.String(), firstHistory.AfterID.String())
	})
}

func TestWorkspaceHistoryByUser(t *testing.T) {
	t.Parallel()
	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		history, err := client.ListWorkspaceHistory(context.Background(), "me", workspace.Name)
		require.NoError(t, err)
		require.NotNil(t, history)
		require.Len(t, history, 0)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, nil)
		coderdtest.AwaitProjectVersionImported(t, client, user.Organization, project.Name, version.Name)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		_, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: version.ID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		history, err := client.ListWorkspaceHistory(context.Background(), "me", workspace.Name)
		require.NoError(t, err)
		require.NotNil(t, history)
		require.Len(t, history, 1)
	})
}

func TestWorkspaceHistoryByName(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t)
	user := coderdtest.CreateInitialUser(t, client)
	coderdtest.NewProvisionerDaemon(t, client)
	project := coderdtest.CreateProject(t, client, user.Organization)
	version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, nil)
	coderdtest.AwaitProjectVersionImported(t, client, user.Organization, project.Name, version.Name)
	workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
	history, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
		ProjectVersionID: version.ID,
		Transition:       database.WorkspaceTransitionStart,
	})
	require.NoError(t, err)
	_, err = client.WorkspaceHistory(context.Background(), "me", workspace.Name, history.Name)
	require.NoError(t, err)
}
