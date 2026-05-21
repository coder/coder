package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestPing(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)
		inv, root := clitest.New(t, "ping", workspace.Name)
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

		pty.ExpectMatch("pong from " + workspace.Name)
		cancel()
		<-cmdDone
	})

	t.Run("1Ping", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)
		inv, root := clitest.New(t, "ping", "-n", "1", workspace.Name)
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

		pty.ExpectMatch("pong from " + workspace.Name)
		cancel()
		<-cmdDone
	})

	t.Run("1PingWithTime", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			utc  bool
		}{
			{name: "LocalTime"},      // --time renders the pong response time.
			{name: "UTC", utc: true}, // --utc implies --time, so we expect it to also contain the pong time.
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				client, workspace, agentToken := setupWorkspaceForAgent(t)
				args := []string{"ping", "-n", "1", workspace.Name, "--time"}
				if tc.utc {
					args = append(args, "--utc")
				}

				inv, root := clitest.New(t, args...)
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

				// RFC3339 is the format used to render the pong times.
				rfc3339 := `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?`

				// Validate that dates are rendered as specified.
				if tc.utc {
					rfc3339 += `Z`
				} else {
					rfc3339 += `(?:Z|[+-]\d{2}:\d{2})`
				}

				pty.ExpectRegexMatch(`\[` + rfc3339 + `\] pong from ` + workspace.Name)
				cancel()
				<-cmdDone
			})
		}
	})
}
