package codersdk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
)

func TestHasInitialUser(t *testing.T) {
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

func TestCreateInitialUser(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.CreateInitialUser(context.Background(), coderd.CreateInitialUserRequest{})
		require.Error(t, err)
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_ = coderdtest.CreateInitialUser(t, client)
	})
}

func TestCreateUser(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.CreateUser(context.Background(), coderd.CreateUserRequest{})
		require.Error(t, err)
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_ = coderdtest.CreateInitialUser(t, client)
		_, err := client.CreateUser(context.Background(), coderd.CreateUserRequest{
			Email:    "example@coder.com",
			Username: "something",
			Password: "password",
		})
		require.NoError(t, err)
	})
}

func TestLoginWithPassword(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{})
		require.Error(t, err)
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

func TestLogout(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t)
	err := client.Logout(context.Background())
	require.NoError(t, err)
}

func TestUser(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.User(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_ = coderdtest.CreateInitialUser(t, client)
		_, err := client.User(context.Background(), "")
		require.NoError(t, err)
	})
}

func TestUserOrganizations(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.UserOrganizations(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_ = coderdtest.CreateInitialUser(t, client)
		_, err := client.UserOrganizations(context.Background(), "")
		require.NoError(t, err)
	})
}
