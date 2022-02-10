package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/httpmw"
)

func TestUser(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		has, err := client.HasInitialUser(context.Background())
		require.NoError(t, err)
		require.False(t, has)
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_ = coderdtest.CreateInitialUser(t, client)
		has, err := client.HasInitialUser(context.Background())
		require.NoError(t, err)
		require.True(t, has)
	})
}

func TestPostUser(t *testing.T) {
	t.Parallel()
	t.Run("BadRequest", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.CreateInitialUser(context.Background(), coderd.CreateInitialUserRequest{})
		require.Error(t, err)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_ = coderdtest.CreateInitialUser(t, client)
		_, err := client.CreateInitialUser(context.Background(), coderd.CreateInitialUserRequest{
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
		client := coderdtest.New(t)
		_ = coderdtest.CreateInitialUser(t, client)
	})
}

func TestPostUsers(t *testing.T) {
	t.Parallel()
	t.Run("BadRequest", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.CreateInitialUser(context.Background(), coderd.CreateInitialUserRequest{})
		require.Error(t, err)
	})

	t.Run("Conflicting", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_, err := client.CreateInitialUser(context.Background(), coderd.CreateInitialUserRequest{
			Email:        user.Email,
			Username:     user.Username,
			Password:     "password",
			Organization: "someorg",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_ = coderdtest.CreateInitialUser(t, client)
		_, err := client.CreateUser(context.Background(), coderd.CreateUserRequest{
			Email:    "another@user.org",
			Username: "someone-else",
			Password: "testing",
		})
		require.NoError(t, err)
	})
}

func TestUserByName(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t)
	_ = coderdtest.CreateInitialUser(t, client)
	_, err := client.User(context.Background(), "")
	require.NoError(t, err)
}

func TestOrganizationsByUser(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t)
	_ = coderdtest.CreateInitialUser(t, client)
	orgs, err := client.UserOrganizations(context.Background(), "")
	require.NoError(t, err)
	require.NotNil(t, orgs)
	require.Len(t, orgs, 1)
}

func TestPostLogin(t *testing.T) {
	t.Parallel()
	t.Run("InvalidUser", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
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
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_, err := client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
			Email:    user.Email,
			Password: "badpass",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_, err := client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
			Email:    user.Email,
			Password: user.Password,
		})
		require.NoError(t, err)
	})
}

func TestPostLogout(t *testing.T) {
	t.Parallel()

	t.Run("ClearCookie", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t)
		fullURL, err := client.URL.Parse("/api/v2/logout")
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
