package cli

import (
	"context"
	"fmt"
	"os"
	"time"

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

type SpeedtestResult struct {
	Overall   SpeedtestResultInterval   `json:"overall"`
	Intervals []SpeedtestResultInterval `json:"intervals"`
}

type SpeedtestResultInterval struct {
	StartTimeSeconds float64 `json:"start_time_seconds"`
	EndTimeSeconds   float64 `json:"end_time_seconds"`
	ThroughputMbits  float64 `json:"throughput_mbits"`
}

type speedtestTableItem struct {
	Interval   string `table:"Interval,nosort"`
	Throughput string `table:"Throughput"`
}

func (r *RootCmd) speedtest() *serpent.Command {
	var (
		direct    bool
		duration  time.Duration
		direction string
		pcapFile  string
		formatter = cliui.NewOutputFormatter(
			cliui.ChangeFormatterData(cliui.TableFormat([]speedtestTableItem{}, []string{"Interval", "Throughput"}), func(data any) (any, error) {
				res, ok := data.(SpeedtestResult)
				if !ok {
					// This should never happen
					return "", xerrors.Errorf("expected speedtestResult, got %T", data)
				}
				tableRows := make([]any, len(res.Intervals)+2)
				for i, r := range res.Intervals {
					tableRows[i] = speedtestTableItem{
						Interval:   fmt.Sprintf("%.2f-%.2f sec", r.StartTimeSeconds, r.EndTimeSeconds),
						Throughput: fmt.Sprintf("%.4f Mbits/sec", r.ThroughputMbits),
					}
				}
				tableRows[len(res.Intervals)] = cliui.TableSeparator{}
				tableRows[len(res.Intervals)+1] = speedtestTableItem{
					Interval:   fmt.Sprintf("%.2f-%.2f sec", res.Overall.StartTimeSeconds, res.Overall.EndTimeSeconds),
					Throughput: fmt.Sprintf("%.4f Mbits/sec", res.Overall.ThroughputMbits),
				}
				return tableRows, nil
			}),
			cliui.JSONFormat(),
		)
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

			if direct && r.disableDirect {
				return xerrors.Errorf("--direct (-d) is incompatible with --%s", varDisableDirect)
			}

			_, workspaceAgent, err := getWorkspaceAndAgent(ctx, inv, client, false, inv.Args[0])
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

			opts := &workspacesdk.DialAgentOptions{}
			if r.verbose {
				opts.Logger = inv.Logger.AppendSinks(sloghuman.Sink(inv.Stderr)).Leveled(slog.LevelDebug)
			}
			if r.disableDirect {
				_, _ = fmt.Fprintln(inv.Stderr, "Direct connections disabled.")
				opts.BlockEndpoints = true
			}
			if pcapFile != "" {
				s := capture.New()
				opts.CaptureHook = s.LogPacket
				f, err := os.OpenFile(pcapFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
				if err != nil {
					return err
				}
				defer f.Close()
				unregister := s.RegisterOutput(f)
				defer unregister()
			}
			conn, err := workspacesdk.New(client).
				DialAgent(ctx, workspaceAgent.ID, opts)
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
						cliui.Infof(inv.Stderr, "Waiting for a direct connection... (%dms via %s)", dur.Milliseconds(), peer.Relay)
						continue
					}
					via := peer.Relay
					if via == "" {
						via = "direct"
					}
					cliui.Infof(inv.Stderr, "%dms via %s", dur.Milliseconds(), via)
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
			cliui.Infof(inv.Stderr, "Starting a %ds %s test...", int(duration.Seconds()), tsDir)
			results, err := conn.Speedtest(ctx, tsDir, duration)
			if err != nil {
				return err
			}
			var outputResult SpeedtestResult
			startTime := results[0].IntervalStart
			outputResult.Intervals = make([]SpeedtestResultInterval, len(results)-1)
			for i, r := range results {
				interval := SpeedtestResultInterval{
					StartTimeSeconds: r.IntervalStart.Sub(startTime).Seconds(),
					EndTimeSeconds:   r.IntervalEnd.Sub(startTime).Seconds(),
					ThroughputMbits:  r.MBitsPerSecond(),
				}
				if r.Total {
					interval.StartTimeSeconds = 0
					outputResult.Overall = interval
				} else {
					outputResult.Intervals[i] = interval
				}
			}
			out, err := formatter.Format(inv.Context(), outputResult)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(inv.Stdout, out)
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
	formatter.AttachOptions(&cmd.Options)
	return cmd
}
