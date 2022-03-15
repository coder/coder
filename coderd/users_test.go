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
	"github.com/coder/coder/httpmw"
)

func TestFirstUser(t *testing.T) {
	t.Parallel()
	t.Run("BadRequest", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.CreateFirstUser(context.Background(), coderd.CreateFirstUserRequest{})
		require.Error(t, err)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		_, err := client.CreateFirstUser(context.Background(), coderd.CreateFirstUserRequest{
			Email:        "some@email.com",
			Username:     "exampleuser",
			Password:     "password",
			Organization: "someorg",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
	})
}

func TestPostLogin(t *testing.T) {
	t.Parallel()
	t.Run("InvalidUser", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
			Email:    "my@email.org",
			Password: "password",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("BadPassword", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		req := coderd.CreateFirstUserRequest{
			Email:        "testuser@coder.com",
			Username:     "testuser",
			Password:     "testpass",
			Organization: "testorg",
		}
		_, err := client.CreateFirstUser(context.Background(), req)
		require.NoError(t, err)
		_, err = client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
			Email:    req.Email,
			Password: "badpass",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		req := coderd.CreateFirstUserRequest{
			Email:        "testuser@coder.com",
			Username:     "testuser",
			Password:     "testpass",
			Organization: "testorg",
		}
		_, err := client.CreateFirstUser(context.Background(), req)
		require.NoError(t, err)
		_, err = client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
			Email:    req.Email,
			Password: req.Password,
		})
		require.NoError(t, err)
	})
}

func TestPostLogout(t *testing.T) {
	t.Parallel()

	t.Run("ClearCookie", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		fullURL, err := client.URL.Parse("/api/v2/users/logout")
		require.NoError(t, err, "Server URL should parse successfully")

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, fullURL.String(), nil)
		require.NoError(t, err, "/logout request construction should succeed")

		httpClient := &http.Client{}

		response, err := httpClient.Do(req)
		require.NoError(t, err, "/logout request should succeed")
		response.Body.Close()

		cookies := response.Cookies()
		require.Len(t, cookies, 1, "Exactly one cookie should be returned")

		require.Equal(t, cookies[0].Name, httpmw.AuthCookie, "Cookie should be the auth cookie")
		require.Equal(t, cookies[0].MaxAge, -1, "Cookie should be set to delete")
	})
}

func TestPostUsers(t *testing.T) {
	t.Parallel()
	t.Run("NoAuth", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.CreateUser(context.Background(), coderd.CreateUserRequest{})
		require.Error(t, err)
	})

	t.Run("Conflicting", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		me, err := client.User(context.Background(), "")
		require.NoError(t, err)
		_, err = client.CreateUser(context.Background(), coderd.CreateUserRequest{
			Email:          me.Email,
			Username:       me.Username,
			Password:       "password",
			OrganizationID: "someorg",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("OrganizationNotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		_, err := client.CreateUser(context.Background(), coderd.CreateUserRequest{
			OrganizationID: "not-exists",
			Email:          "another@user.org",
			Username:       "someone-else",
			Password:       "testing",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("OrganizationNoAccess", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)
		other := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		org, err := other.CreateOrganization(context.Background(), "", coderd.CreateOrganizationRequest{
			Name: "another",
		})
		require.NoError(t, err)

		_, err = client.CreateUser(context.Background(), coderd.CreateUserRequest{
			Email:          "some@domain.com",
			Username:       "anotheruser",
			Password:       "testing",
			OrganizationID: org.ID,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, err := client.CreateUser(context.Background(), coderd.CreateUserRequest{
			OrganizationID: user.OrganizationID,
			Email:          "another@user.org",
			Username:       "someone-else",
			Password:       "testing",
		})
		require.NoError(t, err)
	})
}

func TestUserByName(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	_, err := client.User(context.Background(), "")
	require.NoError(t, err)
}

func TestOrganizationsByUser(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	orgs, err := client.OrganizationsByUser(context.Background(), "")
	require.NoError(t, err)
	require.NotNil(t, orgs)
	require.Len(t, orgs, 1)
}

func TestOrganizationByUserAndName(t *testing.T) {
	t.Parallel()
	t.Run("NoExist", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		_, err := client.OrganizationByName(context.Background(), "", "nothing")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("NoMember", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)
		other := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		org, err := other.CreateOrganization(context.Background(), "", coderd.CreateOrganizationRequest{
			Name: "another",
		})
		require.NoError(t, err)
		_, err = client.OrganizationByName(context.Background(), "", org.Name)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		org, err := client.Organization(context.Background(), user.OrganizationID)
		require.NoError(t, err)
		_, err = client.OrganizationByName(context.Background(), "", org.Name)
		require.NoError(t, err)
	})
}

func TestPostOrganizationsByUser(t *testing.T) {
	t.Parallel()
	t.Run("Conflict", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		org, err := client.Organization(context.Background(), user.OrganizationID)
		require.NoError(t, err)
		_, err = client.CreateOrganization(context.Background(), "", coderd.CreateOrganizationRequest{
			Name: org.Name,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		_, err := client.CreateOrganization(context.Background(), "", coderd.CreateOrganizationRequest{
			Name: "new",
		})
		require.NoError(t, err)
	})
}

func TestPostAPIKey(t *testing.T) {
	t.Parallel()
	t.Run("InvalidUser", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		client.SessionToken = ""
		_, err := client.CreateAPIKey(context.Background(), "")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		apiKey, err := client.CreateAPIKey(context.Background(), "")
		require.NotNil(t, apiKey)
		require.GreaterOrEqual(t, len(apiKey.Key), 2)
		require.NoError(t, err)
	})
}

func TestPostWorkspacesByUser(t *testing.T) {
	t.Parallel()
	t.Run("InvalidProject", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
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
		first := coderdtest.CreateFirstUser(t, client)

		other := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		org, err := other.CreateOrganization(context.Background(), "", coderd.CreateOrganizationRequest{
			Name: "another",
		})
		require.NoError(t, err)
		version := coderdtest.CreateProjectVersion(t, other, org.ID, nil)
		project := coderdtest.CreateProject(t, other, org.ID, version.ID)

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
		user := coderdtest.CreateFirstUser(t, client)
		job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
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
		user := coderdtest.CreateFirstUser(t, client)
		job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
		_ = coderdtest.CreateWorkspace(t, client, "", project.ID)
	})
}

func TestWorkspacesByUser(t *testing.T) {
	t.Parallel()
	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		_, err := client.WorkspacesByUser(context.Background(), "")
		require.NoError(t, err)
	})
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
		_ = coderdtest.CreateWorkspace(t, client, "", project.ID)
		workspaces, err := client.WorkspacesByUser(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, workspaces, 1)
	})
}

func TestWorkspaceByUserAndName(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		_, err := client.WorkspaceByName(context.Background(), "", "something")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})
	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
		_, err := client.WorkspaceByName(context.Background(), "", workspace.Name)
		require.NoError(t, err)
	})
}
