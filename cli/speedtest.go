package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"golang.org/x/xerrors"
	tsspeedtest "tailscale.com/net/speedtest"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) speedtest() *clibase.Cmd {
	var (
		direct    bool
		duration  time.Duration
		direction string
	)
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "speedtest <workspace>",
		Short:       "Run upload and download tests from your machine to a workspace",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			workspace, workspaceAgent, err := getWorkspaceAndAgent(ctx, inv, client, codersdk.Me, inv.Args[0])
			if err != nil {
				return err
			}

			err = cliui.Agent(ctx, inv.Stderr, cliui.AgentOptions{
				WorkspaceName: workspace.Name,
				Fetch: func(ctx context.Context) (codersdk.WorkspaceAgent, error) {
					return client.WorkspaceAgent(ctx, workspaceAgent.ID)
				},
				Wait: false,
			})
			if err != nil && !xerrors.Is(err, cliui.AgentStartError) {
				return xerrors.Errorf("await agent: %w", err)
			}
			logger, ok := LoggerFromContext(ctx)
			if !ok {
				logger = slog.Make(sloghuman.Sink(inv.Stderr))
			}
			if r.verbose {
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
					dur, p2p, _, err := conn.Ping(ctx)
					if err != nil {
						continue
					}
					status := conn.Status()
					if len(status.Peers()) != 1 {
						continue
					}
					peer := status.Peer[status.Peers()[0]]
					if !p2p && direct {
						cliui.Infof(inv.Stdout, "Waiting for a direct connection... (%dms via %s)\n", dur.Milliseconds(), peer.Relay)
						continue
					}
					via := peer.Relay
					if via == "" {
						via = "direct"
					}
					cliui.Infof(inv.Stdout, "%dms via %s\n", dur.Milliseconds(), via)
					break
				}
			} else {
				conn.AwaitReachable(ctx)
			}
			var tsDir tsspeedtest.Direction
			switch direction {
			case "up":
				tsDir = tsspeedtest.Upload
			case "down":
				tsDir = tsspeedtest.Download
			default:
				return xerrors.Errorf("invalid direction: %q", direction)
			}
			cliui.Infof(inv.Stdout, "Starting a %ds %s test...\n", int(duration.Seconds()), tsDir)
			results, err := conn.Speedtest(ctx, tsDir, duration)
			if err != nil {
				return err
			}
			tableWriter := cliui.Table()
			tableWriter.AppendHeader(table.Row{"Interval", "Throughput"})
			startTime := results[0].IntervalStart
			for _, r := range results {
				if r.Total {
					tableWriter.AppendSeparator()
				}
				tableWriter.AppendRow(table.Row{
					fmt.Sprintf("%.2f-%.2f sec", r.IntervalStart.Sub(startTime).Seconds(), r.IntervalEnd.Sub(startTime).Seconds()),
					fmt.Sprintf("%.4f Mbits/sec", r.MBitsPerSecond()),
				})
			}
			_, err = fmt.Fprintln(inv.Stdout, tableWriter.Render())
			return err
		},
	}
	cmd.Options = clibase.OptionSet{
		{
			Description:   "Specifies whether to wait for a direct connection before testing speed.",
			Flag:          "direct",
			FlagShorthand: "d",

			Value: clibase.BoolOf(&direct),
		},
		{
			Description: "Specifies whether to run in reverse mode where the client receives and the server sends.",
			Flag:        "direction",
			Default:     "down",
			Value:       clibase.EnumOf(&direction, "up", "down"),
		},
		{
			Description:   "Specifies the duration to monitor traffic.",
			Flag:          "time",
			FlagShorthand: "t",
			Default:       tsspeedtest.DefaultDuration.String(),
			Value:         clibase.DurationOf(&duration),
		},
	}
	return cmd
}
