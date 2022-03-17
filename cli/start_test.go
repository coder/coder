package cli_test

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/codersdk"
)

func TestStart(t *testing.T) {
	t.Parallel()
	t.Run("Production", func(t *testing.T) {
		ctx, cancelFunc := context.WithCancel(context.Background())
		go cancelFunc()
		root, _ := clitest.New(t, "start", "--address", ":0")
		err := root.ExecuteContext(ctx)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("Development", func(t *testing.T) {
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		root, cfg := clitest.New(t, "start", "--dev", "--tunnel=false", "--address", ":0")
		go func() {
			err := root.ExecuteContext(ctx)
			require.ErrorIs(t, err, context.Canceled)
		}()
		var accessURL string
		require.Eventually(t, func() bool {
			var err error
			accessURL, err = cfg.URL().Read()
			return err == nil
		}, 15*time.Second, 25*time.Millisecond)
		// Verify that authentication was properly set in dev-mode.
		token, err := cfg.Session().Read()
		require.NoError(t, err)
		parsed, err := url.Parse(accessURL)
		require.NoError(t, err)
		client := codersdk.New(parsed)
		client.SessionToken = token
		_, err = client.User(ctx, "")
		require.NoError(t, err)
	})
}
