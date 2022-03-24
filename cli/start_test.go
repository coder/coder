package cli_test

import (
	"context"
	"net/url"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database/postgres"
)

func TestStart(t *testing.T) {
	t.Parallel()
	t.Run("Production", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS != "linux" || testing.Short() {
			// Skip on non-Linux because it spawns a PostgreSQL instance.
			t.SkipNow()
		}
		connectionURL, closeFunc, err := postgres.Open()
		require.NoError(t, err)
		defer closeFunc()
		ctx, cancelFunc := context.WithCancel(context.Background())
		done := make(chan struct{})
		root, cfg := clitest.New(t, "start", "--address", ":0", "--postgres-url", connectionURL)
		go func() {
			defer close(done)
			err = root.ExecuteContext(ctx)
			require.ErrorIs(t, err, context.Canceled)
		}()
		var client *codersdk.Client
		require.Eventually(t, func() bool {
			rawURL, err := cfg.URL().Read()
			if err != nil {
				return false
			}
			accessURL, err := url.Parse(rawURL)
			require.NoError(t, err)
			client = codersdk.New(accessURL)
			return true
		}, 15*time.Second, 25*time.Millisecond)
		_, err = client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
			Email:        "some@one.com",
			Username:     "example",
			Password:     "password",
			Organization: "example",
		})
		require.NoError(t, err)
		cancelFunc()
		<-done
	})
	t.Run("Development", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		root, cfg := clitest.New(t, "start", "--dev", "--tunnel=false", "--address", ":0")
		go func() {
			err := root.ExecuteContext(ctx)
			require.ErrorIs(t, err, context.Canceled)
		}()
		var token string
		require.Eventually(t, func() bool {
			var err error
			token, err = cfg.Session().Read()
			return err == nil
		}, 15*time.Second, 25*time.Millisecond)
		// Verify that authentication was properly set in dev-mode.
		accessURL, err := cfg.URL().Read()
		require.NoError(t, err)
		parsed, err := url.Parse(accessURL)
		require.NoError(t, err)
		client := codersdk.New(parsed)
		client.SessionToken = token
		_, err = client.User(ctx, "")
		require.NoError(t, err)
	})
}
