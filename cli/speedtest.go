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
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func speedtest() *cobra.Command {
	var (
		direct   bool
		duration time.Duration
		reverse  bool
	)
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "speedtest <workspace>",
		Args:        cobra.ExactArgs(1),
		Short:       "Run upload and download tests from your machine to a workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

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
			logger := slog.Make(sloghuman.Sink(cmd.ErrOrStderr()))
			if cliflag.IsSetBool(cmd, varVerbose) {
				logger = logger.Leveled(slog.LevelDebug)
			}
			conn, err := client.DialWorkspaceAgent(ctx, workspaceAgent.ID, &codersdk.DialWorkspaceAgentOptions{
				Logger: logger,
			})
			if err != nil {
				return err
			}
			defer conn.Close()
			if direct {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-ticker.C:
					}
					dur, err := conn.Ping(ctx)
					if err != nil {
						continue
					}
					status := conn.Status()
					if len(status.Peers()) != 1 {
						continue
					}
					peer := status.Peer[status.Peers()[0]]
					if peer.CurAddr == "" && direct {
						cmd.Printf("Waiting for a direct connection... (%dms via %s)\n", dur.Milliseconds(), peer.Relay)
						continue
					}
					via := peer.Relay
					if via == "" {
						via = "direct"
					}
					cmd.Printf("%dms via %s\n", dur.Milliseconds(), via)
					break
				}
			} else {
				conn.AwaitReachable(ctx)
			}
			dir := tsspeedtest.Download
			if reverse {
				dir = tsspeedtest.Upload
			}
			cmd.Printf("Starting a %ds %s test...\n", int(duration.Seconds()), dir)
			results, err := conn.Speedtest(ctx, dir, duration)
			if err != nil {
				return err
			}
			tableWriter := cliui.Table()
			tableWriter.AppendHeader(table.Row{"Interval", "Transfer", "Bandwidth"})
			startTime := results[0].IntervalStart
			for _, r := range results {
				if r.Total {
					tableWriter.AppendSeparator()
				}
				tableWriter.AppendRow(table.Row{
					fmt.Sprintf("%.2f-%.2f sec", r.IntervalStart.Sub(startTime).Seconds(), r.IntervalEnd.Sub(startTime).Seconds()),
					fmt.Sprintf("%.4f MBits", r.MegaBits()),
					fmt.Sprintf("%.4f Mbits/sec", r.MBitsPerSecond()),
				})
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), tableWriter.Render())
			return err
		},
	}
	cliflag.BoolVarP(cmd.Flags(), &direct, "direct", "d", "", false,
		"Specifies whether to wait for a direct connection before testing speed.")
	cliflag.BoolVarP(cmd.Flags(), &reverse, "reverse", "r", "", false,
		"Specifies whether to run in reverse mode where the client receives and the server sends.")
	cmd.Flags().DurationVarP(&duration, "time", "t", tsspeedtest.DefaultDuration,
		"Specifies the duration to monitor traffic.")
	return cmd
}
