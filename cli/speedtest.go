package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"golang.org/x/xerrors"
	tsspeedtest "tailscale.com/net/speedtest"
	"tailscale.com/wgengine/capture"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) speedtest() *serpent.Command {
	var (
		direct    bool
		duration  time.Duration
		direction string
		pcapFile  string
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "speedtest <workspace>",
		Short:       "Run upload and download tests from your machine to a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			_, workspaceAgent, err := getWorkspaceAndAgent(ctx, inv, client, false, codersdk.Me, inv.Args[0])
			if err != nil {
				return err
			}

			err = cliui.Agent(ctx, inv.Stderr, workspaceAgent.ID, cliui.AgentOptions{
				Fetch: client.WorkspaceAgent,
				Wait:  false,
			})
			if err != nil {
				return xerrors.Errorf("await agent: %w", err)
			}

			logger := inv.Logger.AppendSinks(sloghuman.Sink(inv.Stderr))
			if r.verbose {
				logger = logger.Leveled(slog.LevelDebug)
			}

			if r.disableDirect {
				_, _ = fmt.Fprintln(inv.Stderr, "Direct connections disabled.")
			}
			conn, err := workspacesdk.New(client).
				DialAgent(ctx, workspaceAgent.ID, &workspacesdk.DialAgentOptions{
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
						cliui.Infof(inv.Stdout, "Waiting for a direct connection... (%dms via %s)", dur.Milliseconds(), peer.Relay)
						continue
					}
					via := peer.Relay
					if via == "" {
						via = "direct"
					}
					cliui.Infof(inv.Stdout, "%dms via %s", dur.Milliseconds(), via)
					break
				}
			} else {
				conn.AwaitReachable(ctx)
			}

			if pcapFile != "" {
				s := capture.New()
				conn.InstallCaptureHook(s.LogPacket)
				f, err := os.OpenFile(pcapFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
				if err != nil {
					return err
				}
				defer f.Close()
				unregister := s.RegisterOutput(f)
				defer unregister()
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
			cliui.Infof(inv.Stdout, "Starting a %ds %s test...", int(duration.Seconds()), tsDir)
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
	cmd.Options = serpent.OptionSet{
		{
			Description:   "Specifies whether to wait for a direct connection before testing speed.",
			Flag:          "direct",
			FlagShorthand: "d",

			Value: serpent.BoolOf(&direct),
		},
		{
			Description: "Specifies whether to run in reverse mode where the client receives and the server sends.",
			Flag:        "direction",
			Default:     "down",
			Value:       serpent.EnumOf(&direction, "up", "down"),
		},
		{
			Description:   "Specifies the duration to monitor traffic.",
			Flag:          "time",
			FlagShorthand: "t",
			Default:       tsspeedtest.DefaultDuration.String(),
			Value:         serpent.DurationOf(&duration),
		},
		{
			Description: "Specifies a file to write a network capture to.",
			Flag:        "pcap-file",
			Default:     "",
			Value:       serpent.StringOf(&pcapFile),
		},
	}
	return cmd
}
