package coderd_test

import (
	"archive/tar"
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
)

func TestWorkspaces(t *testing.T) {
	t.Parallel()

	t.Run("ListNone", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_ = server.RandomInitialUser(t)
		workspaces, err := server.Client.WorkspacesByUser(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, workspaces, 0)
	})

	setupProjectAndWorkspace := func(t *testing.T, client *codersdk.Client, user coderd.CreateInitialUserRequest) (coderd.Project, coderd.Workspace) {
		project, err := client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "banana",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		workspace, err := client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			Name:      "hiii",
			ProjectID: project.ID,
		})
		require.NoError(t, err)
		return project, workspace
	}

	setupProjectVersion := func(t *testing.T, client *codersdk.Client, user coderd.CreateInitialUserRequest, project coderd.Project) coderd.ProjectHistory {
		var buffer bytes.Buffer
		writer := tar.NewWriter(&buffer)
		err := writer.WriteHeader(&tar.Header{
			Name: "file",
			Size: 1 << 10,
		})
		require.NoError(t, err)
		_, err = writer.Write(make([]byte, 1<<10))
		require.NoError(t, err)
		projectHistory, err := client.CreateProjectHistory(context.Background(), user.Organization, project.Name, coderd.CreateProjectVersionRequest{
			StorageMethod: database.ProjectStorageMethodInlineArchive,
			StorageSource: buffer.Bytes(),
		})
		require.NoError(t, err)
		return projectHistory
	}

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, _ = setupProjectAndWorkspace(t, server.Client, user)
		workspaces, err := server.Client.WorkspacesByUser(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, workspaces, 1)
	})

	t.Run("ListNoneForProject", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "banana",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		workspaces, err := server.Client.WorkspacesByProject(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.Len(t, workspaces, 0)
	})

	t.Run("ListForProject", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, _ := setupProjectAndWorkspace(t, server.Client, user)
		workspaces, err := server.Client.WorkspacesByProject(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.Len(t, workspaces, 1)
	})

	t.Run("CreateInvalidInput", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "banana",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		_, err = server.Client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			ProjectID: project.ID,
			Name:      "$$$",
		})
		require.Error(t, err)
	})

	t.Run("CreateInvalidProject", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_ = server.RandomInitialUser(t)
		_, err := server.Client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			ProjectID: uuid.New(),
			Name:      "moo",
		})
		require.Error(t, err)
	})

	t.Run("CreateNotInProjectOrganization", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		initial := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), initial.Organization, coderd.CreateProjectRequest{
			Name:        "banana",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		_, err = server.Client.CreateUser(context.Background(), coderd.CreateUserRequest{
			Email:    "hello@ok.io",
			Username: "example",
			Password: "wowowow",
		})
		require.NoError(t, err)
		token, err := server.Client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
			Email:    "hello@ok.io",
			Password: "wowowow",
		})
		require.NoError(t, err)
		err = server.Client.SetSessionToken(token.SessionToken)
		require.NoError(t, err)
		_, err = server.Client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			ProjectID: project.ID,
			Name:      "moo",
		})
		require.Error(t, err)
	})

	t.Run("CreateAlreadyExists", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, workspace := setupProjectAndWorkspace(t, server.Client, user)
		_, err := server.Client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			Name:      workspace.Name,
			ProjectID: project.ID,
		})
		require.Error(t, err)
	})

	t.Run("Single", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, workspace := setupProjectAndWorkspace(t, server.Client, user)
		_, err := server.Client.Workspace(context.Background(), "", workspace.Name)
		require.NoError(t, err)
	})

	t.Run("AllHistory", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, workspace := setupProjectAndWorkspace(t, server.Client, user)
		history, err := server.Client.WorkspaceHistory(context.Background(), "", workspace.Name)
		require.NoError(t, err)
		require.Len(t, history, 0)
		projectVersion := setupProjectVersion(t, server.Client, user, project)
		_, err = server.Client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectHistoryID: projectVersion.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)
		history, err = server.Client.WorkspaceHistory(context.Background(), "", workspace.Name)
		require.NoError(t, err)
		require.Len(t, history, 1)
	})

	t.Run("LatestHistory", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, workspace := setupProjectAndWorkspace(t, server.Client, user)
		_, err := server.Client.LatestWorkspaceHistory(context.Background(), "", workspace.Name)
		require.Error(t, err)
		projectVersion := setupProjectVersion(t, server.Client, user, project)
		_, err = server.Client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectHistoryID: projectVersion.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)
		_, err = server.Client.LatestWorkspaceHistory(context.Background(), "", workspace.Name)
		require.NoError(t, err)
	})

	t.Run("CreateHistory", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, workspace := setupProjectAndWorkspace(t, server.Client, user)
		projectHistory := setupProjectVersion(t, server.Client, user, project)

		_, err := server.Client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectHistoryID: projectHistory.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)
	})

	t.Run("CreateHistoryAlreadyInProgress", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, workspace := setupProjectAndWorkspace(t, server.Client, user)
		projectHistory := setupProjectVersion(t, server.Client, user, project)

		_, err := server.Client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectHistoryID: projectHistory.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)

		_, err = server.Client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectHistoryID: projectHistory.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.Error(t, err)
	})

	t.Run("CreateHistoryInvalidProjectVersion", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, workspace := setupProjectAndWorkspace(t, server.Client, user)

		_, err := server.Client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectHistoryID: uuid.New(),
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.Error(t, err)
	})
}
