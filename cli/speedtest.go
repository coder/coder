package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
	tsspeedtest "tailscale.com/net/speedtest"

	"cdr.dev/slog"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func speedtest() *cobra.Command {
	var (
		reverse bool
		timeStr string
	)
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "speedtest <workspace>",
		Short:       "Run a speed test from your machine to the workspace.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			dur, err := time.ParseDuration(timeStr)
			if err != nil {
				return err
			}

			client, err := CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}

			workspace, workspaceAgent, err := getWorkspaceAndAgent(ctx, cmd, client, codersdk.Me, args[0], false)
			if err != nil {
				return err
			}

			err = cliui.Agent(ctx, cmd.ErrOrStderr(), cliui.AgentOptions{
				WorkspaceName: workspace.Name,
				Fetch: func(ctx context.Context) (codersdk.WorkspaceAgent, error) {
					return client.WorkspaceAgent(ctx, workspaceAgent.ID)
				},
			})
			if err != nil {
				return xerrors.Errorf("await agent: %w", err)
			}
			conn, err := client.DialWorkspaceAgentTailnet(ctx, slog.Logger{}, workspaceAgent.ID)
			if err != nil {
				return err
			}
			defer conn.Close()
			_, _ = conn.Ping()
			dir := tsspeedtest.Download
			if reverse {
				dir = tsspeedtest.Upload
			}
			cmd.Printf("Starting a %ds %s test...\n", int(dur.Seconds()), dir)
			results, err := conn.Speedtest(dir, dur)
			if err != nil {
				return err
			}
			tableWriter := cliui.Table()
			tableWriter.AppendHeader(table.Row{"Interval", "Transfer", "Bandwidth"})
			for _, r := range results {
				if r.Total {
					tableWriter.AppendSeparator()
				}
				tableWriter.AppendRow(table.Row{
					fmt.Sprintf("%.2f-%.2f sec", r.IntervalStart.Seconds(), r.IntervalEnd.Seconds()),
					fmt.Sprintf("%.4f MBits", r.MegaBits()),
					fmt.Sprintf("%.4f Mbits/sec", r.MBitsPerSecond()),
				})
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), tableWriter.Render())
			return err
		},
	}
	cliflag.BoolVarP(cmd.Flags(), &reverse, "reverse", "r", "", false,
		"Specifies whether to run in reverse mode where the client receives and the server sends.")
	cliflag.StringVarP(cmd.Flags(), &timeStr, "time", "t", "", "5s",
		"Specifies the duration to monitor traffic.")
	return cmd
}
