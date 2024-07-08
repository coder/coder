package cli_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/enterprise/cli"
	"github.com/coder/coder/v2/testutil"
)

// TestServer runs the enterprise server command
// and waits for /healthz to return "OK".
func TestServer_Single(t *testing.T) {
	t.Parallel()

	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancelFunc()

	var root cli.RootCmd
	cmd, err := root.Command(root.EnterpriseSubcommands())
	require.NoError(t, err)
	inv, cfg := clitest.NewWithCommand(t, cmd,
		"server",
		"--in-memory",
		"--http-address", ":0",
		"--access-url", "http://example.com",
	)
	clitest.Start(t, inv.WithContext(ctx))
	accessURL := waitAccessURL(t, cfg)
	require.Eventually(t, func() bool {
		reqCtx := testutil.Context(t, testutil.IntervalMedium)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, accessURL.String()+"/healthz", nil)
		if err != nil {
			panic(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Log("/healthz not ready yet")
			return false
		}
		defer resp.Body.Close()
		bs, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		return assert.Equal(t, "OK", string(bs))
	}, testutil.WaitShort, testutil.IntervalMedium)
}

func waitAccessURL(t *testing.T, cfg config.Root) *url.URL {
	t.Helper()

	var err error
	var rawURL string
	require.Eventually(t, func() bool {
		rawURL, err = cfg.URL().Read()
		return err == nil && rawURL != ""
	}, testutil.WaitLong, testutil.IntervalFast, "failed to get access URL")

	accessURL, err := url.Parse(rawURL)
	require.NoError(t, err, "failed to parse access URL")

	return accessURL
}
