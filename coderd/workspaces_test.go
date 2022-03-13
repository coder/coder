package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestWorkspace(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
	project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
	workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
	_, err := client.Workspace(context.Background(), workspace.ID)
	require.NoError(t, err)
}

func TestWorkspaceBuilds(t *testing.T) {
	t.Parallel()
	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
		_, err := client.Workspace(context.Background(), workspace.ID)
		require.NoError(t, err)
		_, err = client.WorkspaceBuilds(context.Background(), workspace.ID)
		require.NoError(t, err)
	})
	t.Run("Single", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitProjectVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		_, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		_, err = client.WorkspaceBuilds(context.Background(), workspace.ID)
		require.NoError(t, err)
	})
}

func TestPostWorkspaceBuild(t *testing.T) {
	t.Parallel()
	t.Run("NoProjectVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		_, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
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
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, &echo.Responses{
			Provision: []*proto.Provision_Response{{}},
		})
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitProjectVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		_, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
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
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitProjectVersionJob(t, client, version.ID)
		// Close here so workspace build doesn't process!
		closeDaemon.Close()
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		_, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		_, err = client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
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
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitProjectVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		firstBuild, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJob(t, client, firstBuild.ID)
		secondBuild, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		require.Equal(t, firstBuild.ID.String(), secondBuild.BeforeID.String())

		firstBuild, err = client.WorkspaceBuild(context.Background(), firstBuild.ID)
		require.NoError(t, err)
		require.Equal(t, secondBuild.ID.String(), firstBuild.AfterID.String())
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitProjectVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		firstBuild, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJob(t, client, firstBuild.ID)
		secondBuild, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionDelete,
		})
		require.NoError(t, err)
		require.Equal(t, firstBuild.ID.String(), secondBuild.BeforeID.String())
		coderdtest.AwaitWorkspaceBuildJob(t, client, secondBuild.ID)

		workspaces, err := client.WorkspacesByUser(context.Background(), user.UserID)
		require.NoError(t, err)
		require.Len(t, workspaces, 0)
	})
}

func TestWorkspaceBuildLatest(t *testing.T) {
	t.Parallel()
	t.Run("None", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitProjectVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		_, err := client.WorkspaceBuildLatest(context.Background(), workspace.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitProjectVersionJob(t, client, version.ID)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		_, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		_, err = client.WorkspaceBuildLatest(context.Background(), workspace.ID)
		require.NoError(t, err)
	})
}

func TestWorkspaceBuildByName(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitProjectVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		_, err := client.WorkspaceBuildByName(context.Background(), workspace.ID, "something")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitProjectVersionJob(t, client, version.ID)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		build, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		_, err = client.WorkspaceBuildByName(context.Background(), workspace.ID, build.Name)
		require.NoError(t, err)
	})
}
