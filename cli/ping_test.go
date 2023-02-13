package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

func TestPing(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t, nil)
		cmd, root := clitest.New(t, "ping", workspace.Name)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetErr(pty.Output())
		cmd.SetOut(pty.Output())

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(agentToken)
		agentCloser := agent.New(agent.Options{
			Client: agentClient,
			Logger: slogtest.Make(t, nil).Named("agent"),
		})
		defer func() {
			_ = agentCloser.Close()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		cmdDone := tGo(t, func() {
			err := cmd.ExecuteContext(ctx)
			assert.NoError(t, err)
		})

		pty.ExpectMatch("pong from " + workspace.Name)
		cancel()
		<-cmdDone
	})
}
