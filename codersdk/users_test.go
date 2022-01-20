package codersdk_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/stretchr/testify/require"
)

func TestUsers(t *testing.T) {
	t.Run("MultipleInitial", func(t *testing.T) {
		server := coderdtest.New(t)
		_, err := server.Client.CreateInitialUser(context.Background(), coderd.CreateUserRequest{
			Email:    "wowie@coder.com",
			Username: "tester",
			Password: "moo",
		})
		var cerr *codersdk.Error
		require.ErrorAs(t, err, &cerr)
		require.Equal(t, cerr.StatusCode(), http.StatusConflict)
		require.Greater(t, len(cerr.Error()), 0)
	})

	t.Run("Get", func(t *testing.T) {
		server := coderdtest.New(t)
		_, err := server.Client.User(context.Background(), "")
		require.NoError(t, err)
	})
}
