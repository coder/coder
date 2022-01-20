package coderd_test

import (
	"context"
	"testing"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/stretchr/testify/require"
)

func TestUsers(t *testing.T) {
	t.Parallel()

	t.Run("Authenticated", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_, err := server.Client.User(context.Background(), "")
		require.NoError(t, err)
	})

	t.Run("CreateMultipleInitial", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_, err := server.Client.CreateInitialUser(context.Background(), coderd.CreateUserRequest{
			Email:    "dummy@coder.com",
			Username: "fake",
			Password: "password",
		})
		require.Error(t, err)
	})

	t.Run("LoginNoEmail", func(t *testing.T) {
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
		user, err := server.Client.User(context.Background(), "")
		require.NoError(t, err)

		_, err = server.Client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
			Email:    user.Email,
			Password: "bananas",
		})
		require.Error(t, err)
	})
}
