package cli_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestOpen(t *testing.T) {
	t.Parallel()

	t.Run("VS Code Local", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t, func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Directory = "/tmp"
			agents[0].Name = "agent1"
			return agents
		})

		_ = agenttest.New(t, client.URL, agentToken)
		_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		inv, root := clitest.New(t, "open", "vscode", "--test.no-open", workspace.Name)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		inv.Stdin = pty.Input()
		inv.Stdout = pty.Output()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err)
		})

		me, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)

		// --test.no-open forces the command to print the URI.
		line := pty.ReadLine(ctx)
		u, err := url.ParseRequestURI(line)
		require.NoError(t, err, "line: %q", line)

		qp := u.Query()
		assert.Equal(t, client.URL.String(), qp.Get("url"))
		assert.Equal(t, me.Username, qp.Get("owner"))
		assert.Equal(t, workspace.Name, qp.Get("workspace"))
		assert.Equal(t, "agent1", qp.Get("agent"))
		assert.Contains(t, "tmp", qp.Get("folder")) // Soft check for windows compat.
		assert.Equal(t, "", qp.Get("token"))

		<-cmdDone
	})
	t.Run("VS Code Inside Workspace Prints URI", func(t *testing.T) {
		t.Parallel()

		agentName := "agent1"
		client, workspace, agentToken := setupWorkspaceForAgent(t, func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Directory = "/tmp"
			agents[0].Name = agentName
			return agents
		})

		_ = agenttest.New(t, client.URL, agentToken)
		_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		t.Log(client.SessionToken())

		inv, root := clitest.New(t, "open", "vscode", "--generate-token", workspace.Name)
		clitest.SetupConfig(t, client, root)

		t.Log(root.Session().Read())

		pty := ptytest.New(t)
		inv.Stdin = pty.Input()
		inv.Stdout = pty.Output()

		inv.Environ.Set("CODER", "true")
		inv.Environ.Set("CODER_WORKSPACE_NAME", workspace.Name)
		inv.Environ.Set("CODER_WORKSPACE_AGENT_NAME", agentName)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err)
		})

		me, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)

		line := pty.ReadLine(ctx)
		u, err := url.ParseRequestURI(line)
		require.NoError(t, err, "line: %q", line)

		qp := u.Query()
		assert.Equal(t, client.URL.String(), qp.Get("url"))
		assert.Equal(t, me.Username, qp.Get("owner"))
		assert.Equal(t, workspace.Name, qp.Get("workspace"))
		assert.Equal(t, "agent1", qp.Get("agent"))
		assert.Equal(t, "/tmp", qp.Get("folder"))
		assert.NotEmpty(t, qp.Get("token"))

		<-cmdDone
	})
}
