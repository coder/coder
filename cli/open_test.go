package cli_test

import (
	"context"
	"net/url"
	"strings"
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

	t.Run("VSCode", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t, func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Directory = "/tmp"
			agents[0].Name = "agent1"
			return agents
		})

		inv, root := clitest.New(t, "open", "vscode", "--test.no-open", workspace.Name)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		inv.Stdin = pty.Input()
		inv.Stderr = pty.Output()
		inv.Stdout = pty.Output()

		_ = agenttest.New(t, client.URL, agentToken)
		_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err)
		})

		me, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)

		line := pty.ReadLine(ctx)

		// Opening vscode://coder.coder-remote/open?...
		parts := strings.Split(line, " ")
		require.Len(t, parts, 2)
		require.Contains(t, parts[1], "vscode://")
		u, err := url.ParseRequestURI(parts[1])
		require.NoError(t, err)

		qp := u.Query()
		assert.Equal(t, client.URL.String(), qp.Get("url"))
		assert.Equal(t, me.Username, qp.Get("owner"))
		assert.Equal(t, workspace.Name, qp.Get("workspace"))
		assert.Equal(t, "agent1", qp.Get("agent"))
		assert.Equal(t, "/tmp", qp.Get("folder"))

		cancel()
		<-cmdDone
	})
}
