package codersdk_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/codersdk"
)

var (
	serverURL = &url.URL{
		Scheme: "https",
		Host:   "coder.com",
	}
)

func TestClient(t *testing.T) {
	t.Parallel()
	t.Run("SetClearSessionToken", func(t *testing.T) {
		t.Parallel()
		client := codersdk.New(serverURL)
		err := client.SetSessionToken("test-session")
		require.NoError(t, err, "Setting a session token should be successful")

		err = client.ClearSessionToken()
		require.NoError(t, err, "Clearing a session token should be successful")
	})
}
