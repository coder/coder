package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
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
}
