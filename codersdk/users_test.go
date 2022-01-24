package codersdk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
)

func TestUsers(t *testing.T) {
	t.Run("CreateInitial", func(t *testing.T) {
		server := coderdtest.New(t)
		_, err := server.Client.CreateInitialUser(context.Background(), coderd.CreateInitialUserRequest{
			Email:        "wowie@coder.com",
			Organization: "somethin",
			Username:     "tester",
			Password:     "moo",
		})
		require.NoError(t, err)
	})

	t.Run("NoUser", func(t *testing.T) {
		server := coderdtest.New(t)
		_, err := server.Client.User(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("User", func(t *testing.T) {
		server := coderdtest.New(t)
		_ = server.RandomInitialUser(t)
		_, err := server.Client.User(context.Background(), "")
		require.NoError(t, err)
	})

	t.Run("UserOrganizations", func(t *testing.T) {
		server := coderdtest.New(t)
		_ = server.RandomInitialUser(t)
		orgs, err := server.Client.UserOrganizations(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, orgs, 1)
	})
}
