package cli_test

import (
	"bytes"
	"fmt"
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
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		ctx, cancelFunc := testutil.Context(t)
		defer cancelFunc()
		done := make(chan error)
		go func() {
			err := cmd.ExecuteContext(ctx)
			done <- err
		}()

		var line string
		require.Eventually(t, func() bool {
			fmt.Printf("Looking for port!\n")
			var err error
			line, err = buf.ReadString('\n')
			return err == nil
		}, testutil.WaitMedium, testutil.IntervalFast)
		t.Logf("Port: %s\n", line)

		cancelFunc()
		<-done
	})
}
