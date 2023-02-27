package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

func TestSpeedtest(t *testing.T) {
	t.Parallel()
	t.Skip("Flaky test - see https://github.com/coder/coder/issues/6321")
	if testing.Short() {
		t.Skip("This test takes a minimum of 5ms per a hardcoded value in Tailscale!")
	}
	client, workspace, agentToken := setupWorkspaceForAgent(t, nil)
	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(agentToken)
	agentCloser := agent.New(agent.Options{
		Client: agentClient,
		Logger: slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
	})
	defer agentCloser.Close()
	coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	require.Eventually(t, func() bool {
		ws, err := client.Workspace(ctx, workspace.ID)
		if !assert.NoError(t, err) {
			return false
		}
		a := ws.LatestBuild.Resources[0].Agents[0]
		return a.Status == codersdk.WorkspaceAgentConnected &&
			a.LifecycleState == codersdk.WorkspaceAgentLifecycleReady
	}, testutil.WaitLong, testutil.IntervalFast, "agent is not ready")

	cmd, root := clitest.New(t, "speedtest", workspace.Name)
	clitest.SetupConfig(t, client, root)
	pty := ptytest.New(t)
	cmd.SetOut(pty.Output())
	cmd.SetErr(pty.Output())

	ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	ctx = cli.ContextWithLogger(ctx, slogtest.Make(t, nil).Named("speedtest").Leveled(slog.LevelDebug))
	cmdDone := tGo(t, func() {
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)
	})
	<-cmdDone
}
