package cli_test

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/testutil"
)

func TestVSCodeIPC(t *testing.T) {
	t.Parallel()
	// Ensures the vscodeipc command outputs it's running port!
	// This signifies to the caller that it's ready to accept requests.
	t.Run("PortOutputs", func(t *testing.T) {
		t.Parallel()
		client, workspace, _ := setupWorkspaceForAgent(t, nil)
		cmd, _ := clitest.New(t, "vscodeipc", workspace.LatestBuild.Resources[0].Agents[0].ID.String(),
			"--token", client.SessionToken(), "--url", client.URL.String())
		rdr, wtr := io.Pipe()
		cmd.SetOut(wtr)
		ctx, cancelFunc := testutil.Context(t)
		defer cancelFunc()
		done := make(chan error, 1)
		go func() {
			err := cmd.ExecuteContext(ctx)
			done <- err
		}()

		buf := make([]byte, 64)
		require.Eventually(t, func() bool {
			t.Log("Looking for address!")
			var err error
			_, err = rdr.Read(buf)
			return err == nil
		}, testutil.WaitMedium, testutil.IntervalFast)
		t.Logf("Address: %s\n", buf)

		cancelFunc()
		<-done
	})
}
