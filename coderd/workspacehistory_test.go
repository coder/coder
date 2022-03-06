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

func TestPostWorkspaceBuildByUser(t *testing.T) {
	t.Parallel()
	t.Run("NoProjectVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		_, err := client.CreateWorkspaceBuild(context.Background(), "", workspace.Name, coderd.CreateWorkspaceBuildRequest{
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
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, &echo.Responses{
			Provision: []*proto.Provision_Response{{}},
		})
		project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
		coderdtest.AwaitProjectImportJob(t, client, user.OrganizationID, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		_, err := client.CreateWorkspaceBuild(context.Background(), "", workspace.Name, coderd.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})

	t.Run("AlreadyActive", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		closeDaemon := coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
		coderdtest.AwaitProjectImportJob(t, client, user.OrganizationID, job.ID)
		// Close here so workspace build doesn't process!
		closeDaemon.Close()
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		_, err := client.CreateWorkspaceBuild(context.Background(), "", workspace.Name, coderd.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		_, err = client.CreateWorkspaceBuild(context.Background(), "", workspace.Name, coderd.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("UpdatePriorAfterField", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
		coderdtest.AwaitProjectImportJob(t, client, user.OrganizationID, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		firstHistory, err := client.CreateWorkspaceBuild(context.Background(), "", workspace.Name, coderd.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceProvisionJob(t, client, user.OrganizationID, firstHistory.ProvisionJobID)
		secondHistory, err := client.CreateWorkspaceBuild(context.Background(), "", workspace.Name, coderd.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		require.Equal(t, firstHistory.ID.String(), secondHistory.BeforeID.String())

		firstHistory, err = client.WorkspaceBuild(context.Background(), "", workspace.Name, firstHistory.Name)
		require.NoError(t, err)
		require.Equal(t, secondHistory.ID.String(), firstHistory.AfterID.String())
	})
}

func TestWorkspaceBuildByUser(t *testing.T) {
	t.Parallel()
	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		history, err := client.ListWorkspaceBuild(context.Background(), "me", workspace.Name)
		require.NoError(t, err)
		require.NotNil(t, history)
		require.Len(t, history, 0)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
		coderdtest.AwaitProjectImportJob(t, client, user.OrganizationID, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		_, err := client.CreateWorkspaceBuild(context.Background(), "", workspace.Name, coderd.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		history, err := client.ListWorkspaceBuild(context.Background(), "me", workspace.Name)
		require.NoError(t, err)
		require.NotNil(t, history)
		require.Len(t, history, 1)
	})
}

func TestWorkspaceBuildByName(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	coderdtest.NewProvisionerDaemon(t, client)
	job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitProjectImportJob(t, client, user.OrganizationID, job.ID)
	project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
	workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
	history, err := client.CreateWorkspaceBuild(context.Background(), "", workspace.Name, coderd.CreateWorkspaceBuildRequest{
		ProjectVersionID: project.ActiveVersionID,
		Transition:       database.WorkspaceTransitionStart,
	})
	require.NoError(t, err)
	_, err = client.WorkspaceBuild(context.Background(), "me", workspace.Name, history.Name)
	require.NoError(t, err)
}
