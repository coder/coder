package codersdk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
)

func TestUsers(t *testing.T) {
	t.Parallel()
	t.Run("CreateInitial", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.CreateInitialUser(context.Background(), coderd.CreateInitialUserRequest{
			Email:        "wowie@coder.com",
			Organization: "somethin",
			Username:     "tester",
			Password:     "moo",
		})
		require.NoError(t, err)
	})

	t.Run("NoUser", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.User(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("User", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_ = coderdtest.CreateInitialUser(t, client)
		_, err := client.User(context.Background(), "")
		require.NoError(t, err)
	})

	t.Run("UserOrganizations", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_ = coderdtest.CreateInitialUser(t, client)
		orgs, err := client.UserOrganizations(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, orgs, 1)
	})

	t.Run("LogoutIsSuccessful", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_ = coderdtest.CreateInitialUser(t, client)
		err := client.Logout(context.Background())
		require.NoError(t, err)
	})

	t.Run("CreateMultiple", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_ = coderdtest.CreateInitialUser(t, client)
		_, err := client.CreateUser(context.Background(), coderd.CreateUserRequest{
			Email:    "wow@ok.io",
			Username: "example",
			Password: "tomato",
		})
		require.NoError(t, err)
	})
}
