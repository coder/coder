package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
)

func TestPing_AgentSuggestions(t *testing.T) {
	t.Parallel()

	client, workspace, _ := setupWorkspaceForAgent(t, func(agents []*sdkproto.Agent) []*sdkproto.Agent {
		agents[0].Name = "coder"
		return append(agents, &sdkproto.Agent{
			Id:   uuid.NewString(),
			Name: "dev",
			Auth: &sdkproto.Agent_Token{Token: uuid.NewString()},
		})
	})
	qualifiedName := "myuser/" + workspace.Name
	for _, tc := range []struct {
		name  string
		input string
		want  string
		json  bool
	}{
		{name: "Name", input: workspace.Name, want: workspace.Name},
		{name: "OwnerAndName", input: qualifiedName, want: qualifiedName},
		{name: "UUID", input: workspace.ID.String(), want: qualifiedName},
		{name: "JSON", input: workspace.Name, want: workspace.Name, json: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args := []string{"ping", tc.input}
			if tc.json {
				args = append(args, "--num", "1", "--output", "json")
			}
			inv, root := clitest.New(t, args...)
			clitest.SetupConfig(t, client, root)
			var stdout bytes.Buffer
			inv.Stdout = &stdout
			err := inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run()
			require.ErrorContains(t, err, "multiple agents found")
			require.ErrorContains(t, err, "Try running:")
			assert.Contains(t, err.Error(), "$ coder ping "+tc.want+".coder")
			assert.Contains(t, err.Error(), "$ coder ping "+tc.want+".dev")
			assert.NotContains(t, err.Error(), "$ ssh ")
			assert.Empty(t, stdout.String())
		})
	}
}

func TestPing(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)
		inv, root := clitest.New(t, "ping", workspace.Name)
		clitest.SetupConfig(t, client, root)
		stdout := expecter.NewAttachedToInvocation(t, inv)

		_ = agenttest.New(t, client.URL, agentToken)
		_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err)
		})

		stdout.ExpectMatch(ctx, "pong from "+workspace.Name)
		cancel()
		<-cmdDone
	})

	t.Run("1Ping", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)
		inv, root := clitest.New(t, "ping", "-n", "1", workspace.Name)
		clitest.SetupConfig(t, client, root)
		stdout := expecter.NewAttachedToInvocation(t, inv)

		_ = agenttest.New(t, client.URL, agentToken)
		_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err)
		})

		stdout.ExpectMatch(ctx, "pong from "+workspace.Name)
		// The default table output prints the summary after the last ping.
		stdout.ExpectRegexMatch(ctx, `(?i)successful`)
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
				stdout := expecter.NewAttachedToInvocation(t, inv)

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

				stdout.ExpectRegexMatch(ctx, `\[`+rfc3339+`\] pong from `+workspace.Name)
				cancel()
				<-cmdDone
			})
		}
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)
		inv, root := clitest.New(t, "ping", "-n", "1", "-o", "json", workspace.Name)
		clitest.SetupConfig(t, client, root)
		out := bytes.NewBuffer(nil)
		inv.Stdout = out

		_ = agenttest.New(t, client.URL, agentToken)
		_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// stdout must contain nothing but the JSON document; diagnostic
		// and per-attempt lines are redirected to stderr in JSON mode.
		var result cli.PingJSONOutput
		require.NoError(t, json.Unmarshal(out.Bytes(), &result))
		assert.Equal(t, workspace.Name, result.Workspace)
		require.Len(t, result.Pongs, 1)
		assert.Equal(t, 1, result.Pongs[0].Sequence)
		assert.Empty(t, result.Pongs[0].Error)
		require.NotNil(t, result.Pongs[0].LatencyMS)
		require.NotNil(t, result.Pongs[0].Direct)
		assert.Equal(t, 1, result.Summary.Total)
		assert.Equal(t, 1, result.Summary.Successful)
		require.NotNil(t, result.Summary.MinMS)
		require.NotNil(t, result.Summary.AvgMS)
		require.NotNil(t, result.Summary.MaxMS)
	})

	t.Run("JSONRequiresNum", func(t *testing.T) {
		t.Parallel()

		for _, args := range [][]string{
			{"ping", "-o", "json", "my-workspace"},
			{"ping", "-n", "0", "-o", "json", "my-workspace"},
		} {
			inv, _ := clitest.New(t, args...)
			out := bytes.NewBuffer(nil)
			inv.Stdout = out

			err := inv.Run()
			require.ErrorContains(t, err, "--output json requires --num")
			assert.Empty(t, out.String())
		}
	})
}
