package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/httpmw"
)

func TestUsers(t *testing.T) {
	t.Parallel()

	t.Run("Authenticated", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_ = server.RandomInitialUser(t)
		_, err := server.Client.User(context.Background(), "")
		require.NoError(t, err)
	})

	t.Run("CreateMultipleInitial", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_ = server.RandomInitialUser(t)
		_, err := server.Client.CreateInitialUser(context.Background(), coderd.CreateInitialUserRequest{
			Email:        "dummy@coder.com",
			Organization: "bananas",
			Username:     "fake",
			Password:     "password",
		})
		require.Error(t, err)
	})

	t.Run("Login", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, err := server.Client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
			Email:    user.Email,
			Password: user.Password,
		})
		require.NoError(t, err)
	})

	t.Run("LoginInvalidUser", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_, err := server.Client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
			Email:    "hello@io.io",
			Password: "wowie",
		})
		require.Error(t, err)
	})

	t.Run("LoginBadPassword", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, err := server.Client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
			Email:    user.Email,
			Password: "bananas",
		})
		require.Error(t, err)
	})

	t.Run("ListOrganizations", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_ = server.RandomInitialUser(t)
		orgs, err := server.Client.UserOrganizations(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, orgs, 1)
	})

	t.Run("CreateUser", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_ = server.RandomInitialUser(t)
		_, err := server.Client.CreateUser(context.Background(), coderd.CreateUserRequest{
			Email:    "wow@ok.io",
			Username: "tomato",
			Password: "bananas",
		})
		require.NoError(t, err)
	})

	t.Run("CreateUserConflict", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, err := server.Client.CreateUser(context.Background(), coderd.CreateUserRequest{
			Email:    "wow@ok.io",
			Username: user.Username,
			Password: "bananas",
		})
		require.Error(t, err)
	})
}

func TestLogout(t *testing.T) {
	t.Parallel()

	t.Run("LogoutShouldClearCookie", func(t *testing.T) {
		t.Parallel()

		server := coderdtest.New(t)
		fullURL, err := server.URL.Parse("/api/v2/logout")
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
