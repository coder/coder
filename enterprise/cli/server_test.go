package cli_test

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/enterprise/cli"
	"github.com/coder/coder/v2/testutil"
)

// TestServer runs the enterprise server command
// and waits for /healthz to return "OK".
func TestServer(t *testing.T) {
	t.Parallel()

	var root cli.RootCmd
	cmd, err := root.Command(root.EnterpriseSubcommands())
	require.NoError(t, err)
	port := testutil.RandomPort(t)
	inv, _ := clitest.NewWithCommand(t, cmd,
		"server",
		"--in-memory",
		"--http-address", fmt.Sprintf(":%d", port),
		"--access-url", "http://example.com",
	)
	waiter := clitest.StartWithWaiter(t, inv)
	require.Eventually(t, func() bool {
		reqCtx := testutil.Context(t, testutil.IntervalMedium)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, fmt.Sprintf("http://localhost:%d/healthz", port), nil)
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
	waiter.Cancel()
}
