package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

func TestSpeedtest(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("This test takes a minimum of 5ms per a hardcoded value in Tailscale!")
	}
	client, workspace, agentToken := setupWorkspaceForAgent(t)
	agentClient := codersdk.New(client.URL)
	agentClient.SessionToken = agentToken
	agentCloser := agent.New(agent.Options{
		FetchMetadata:              agentClient.WorkspaceAgentMetadata,
		CoordinatorDialer:          agentClient.ListenWorkspaceAgentTailnet,
		Logger:                     slogtest.Make(t, nil).Named("agent"),
		WorkspaceAppHealthReporter: func(context.Context) {},
	})
	defer agentCloser.Close()
	coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)

	cmd, root := clitest.New(t, "speedtest", workspace.Name)
	clitest.SetupConfig(t, client, root)
	pty := ptytest.New(t)
	cmd.SetOut(pty.Output())

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	cmdDone := tGo(t, func() {
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)
	})
	<-cmdDone
}
