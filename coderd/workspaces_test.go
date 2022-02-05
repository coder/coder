package coderd_test

import (
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
		client := coderdtest.New(t)
		_ = coderdtest.NewInitialUser(t, client)
		workspaces, err := client.WorkspacesByUser(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, workspaces, 0)
	})

	setupProjectAndWorkspace := func(t *testing.T, client *codersdk.Client, user coderd.CreateInitialUserRequest) (coderd.Project, coderd.Workspace) {
		project, err := client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "banana",
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		workspace, err := client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			Name:      "example",
			ProjectID: project.ID,
		})
		require.NoError(t, err)
		return project, workspace
	}

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, client)
		_, _ = setupProjectAndWorkspace(t, client, user)
		workspaces, err := client.WorkspacesByUser(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, workspaces, 1)
	})

	t.Run("ListNoneForProject", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, client)
		project, err := client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "banana",
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		workspaces, err := client.WorkspacesByProject(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.Len(t, workspaces, 0)
	})

	t.Run("ListForProject", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, client)
		project, _ := setupProjectAndWorkspace(t, client, user)
		workspaces, err := client.WorkspacesByProject(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.Len(t, workspaces, 1)
	})

	t.Run("CreateInvalidInput", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, client)
		project, err := client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "banana",
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		_, err = client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			ProjectID: project.ID,
			Name:      "$$$",
		})
		require.Error(t, err)
	})

	t.Run("CreateInvalidProject", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_ = coderdtest.NewInitialUser(t, client)
		_, err := client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			ProjectID: uuid.New(),
			Name:      "moo",
		})
		require.Error(t, err)
	})

	t.Run("CreateNotInProjectOrganization", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		initial := coderdtest.NewInitialUser(t, client)
		project, err := client.CreateProject(context.Background(), initial.Organization, coderd.CreateProjectRequest{
			Name:        "banana",
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		_, err = client.CreateUser(context.Background(), coderd.CreateUserRequest{
			Email:    "hello@ok.io",
			Username: "example",
			Password: "password",
		})
		require.NoError(t, err)
		token, err := client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
			Email:    "hello@ok.io",
			Password: "password",
		})
		require.NoError(t, err)
		err = client.SetSessionToken(token.SessionToken)
		require.NoError(t, err)
		_, err = client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			ProjectID: project.ID,
			Name:      "moo",
		})
		require.Error(t, err)
	})

	t.Run("CreateAlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, client)
		project, workspace := setupProjectAndWorkspace(t, client, user)
		_, err := client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			Name:      workspace.Name,
			ProjectID: project.ID,
		})
		require.Error(t, err)
	})

	t.Run("Single", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, client)
		_, workspace := setupProjectAndWorkspace(t, client, user)
		_, err := client.Workspace(context.Background(), "", workspace.Name)
		require.NoError(t, err)
	})
}
