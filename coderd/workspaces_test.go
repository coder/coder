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
)

func TestWorkspaces(t *testing.T) {
	t.Parallel()
	t.Run("ListNone", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateInitialUser(t, client)
		workspaces, err := client.Workspaces(context.Background(), "")
		require.NoError(t, err)
		require.NotNil(t, workspaces)
		require.Len(t, workspaces, 0)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		_ = coderdtest.CreateWorkspace(t, client, "", project.ID)
		workspaces, err := client.Workspaces(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, workspaces, 1)
	})
}

func TestPostWorkspaceByUser(t *testing.T) {
	t.Parallel()
	t.Run("InvalidProject", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateInitialUser(t, client)
		_, err := client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			ProjectID: uuid.New(),
			Name:      "workspace",
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("NoProjectAccess", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)

		anotherUser := coderd.CreateUserRequest{
			Email:    "another@user.org",
			Username: "someuser",
			Password: "somepass",
		}
		_, err := client.CreateUser(context.Background(), anotherUser)
		require.NoError(t, err)
		token, err := client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
			Email:    anotherUser.Email,
			Password: anotherUser.Password,
		})
		require.NoError(t, err)
		client.SessionToken = token.SessionToken
		require.NoError(t, err)

		_, err = client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			ProjectID: project.ID,
			Name:      "workspace",
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
		_, err := client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			ProjectID: project.ID,
			Name:      workspace.Name,
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		_ = coderdtest.CreateWorkspace(t, client, "", project.ID)
	})
}

func TestWorkspaceByUser(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	user := coderdtest.CreateInitialUser(t, client)
	job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
	project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
	workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
	_, err := client.Workspace(context.Background(), "", workspace.Name)
	require.NoError(t, err)
}

func TestWorkspacesByProject(t *testing.T) {
	t.Parallel()
	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		workspaces, err := client.WorkspacesByProject(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.NotNil(t, workspaces)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		_ = coderdtest.CreateWorkspace(t, client, "", project.ID)
		workspaces, err := client.WorkspacesByProject(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.NotNil(t, workspaces)
		require.Len(t, workspaces, 1)
	})
}
