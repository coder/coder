package cli

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) ping() *serpent.Command {
	var (
		pingNum     int64
		pingTimeout time.Duration
		pingWait    time.Duration
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "ping <workspace>",
		Short:       "Ping a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			workspaceName := inv.Args[0]
			_, workspaceAgent, err := getWorkspaceAndAgent(
				ctx, inv, client,
				false, // Do not autostart for a ping.
				codersdk.Me, workspaceName,
			)
			if err != nil {
				return err
			}

			logger := inv.Logger
			if r.verbose {
				logger = logger.AppendSinks(sloghuman.Sink(inv.Stdout)).Leveled(slog.LevelDebug)
			}

			if r.disableDirect {
				_, _ = fmt.Fprintln(inv.Stderr, "Direct connections disabled.")
			}
			conn, err := workspacesdk.New(client).
				DialAgent(ctx, workspaceAgent.ID, &workspacesdk.DialAgentOptions{
					Logger:         logger,
					BlockEndpoints: r.disableDirect,
				})
			if err != nil {
				return err
			}
			defer conn.Close()

			derpMap := conn.DERPMap()
			_ = derpMap

			n := 0
			didP2p := false
			start := time.Now()
			for {
				if n > 0 {
					time.Sleep(pingWait)
				}
				n++

				ctx, cancel := context.WithTimeout(ctx, pingTimeout)
				dur, p2p, pong, err := conn.Ping(ctx)
				cancel()
				if err != nil {
					if xerrors.Is(err, context.DeadlineExceeded) {
						_, _ = fmt.Fprintf(inv.Stdout, "ping to %q timed out \n", workspaceName)
						if n == int(pingNum) {
							return nil
						}
						continue
					}
					if xerrors.Is(err, context.Canceled) {
						return nil
					}

					if err.Error() == "no matching peer" {
						continue
					}

					_, _ = fmt.Fprintf(inv.Stdout, "ping to %q failed %s\n", workspaceName, err.Error())
					if n == int(pingNum) {
						return nil
					}
					continue
				}

				dur = dur.Round(time.Millisecond)
				var via string
				if p2p {
					if !didP2p {
						_, _ = fmt.Fprintln(inv.Stdout, "p2p connection established in",
							pretty.Sprint(cliui.DefaultStyles.DateTimeStamp, time.Since(start).Round(time.Millisecond).String()),
						)
					}
					didP2p = true

					via = fmt.Sprintf("%s via %s",
						pretty.Sprint(cliui.DefaultStyles.Fuchsia, "p2p"),
						pretty.Sprint(cliui.DefaultStyles.Code, pong.Endpoint),
					)
				} else {
					derpName := "unknown"
					derpRegion, ok := derpMap.Regions[pong.DERPRegionID]
					if ok {
						derpName = derpRegion.RegionName
					}
					via = fmt.Sprintf("%s via %s",
						pretty.Sprint(cliui.DefaultStyles.Fuchsia, "proxied"),
						pretty.Sprint(cliui.DefaultStyles.Code, fmt.Sprintf("DERP(%s)", derpName)),
					)
				}

				_, _ = fmt.Fprintf(inv.Stdout, "pong from %s %s in %s\n",
					pretty.Sprint(cliui.DefaultStyles.Keyword, workspaceName),
					via,
					pretty.Sprint(cliui.DefaultStyles.DateTimeStamp, dur.String()),
				)

				if n == int(pingNum) {
					diags := conn.GetPeerDiagnostics()
					cliui.PeerDiagnostics(inv.Stdout, diags)
					return nil
				}
			}
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "wait",
			Description: "Specifies how long to wait between pings.",
			Default:     "1s",
			Value:       serpent.DurationOf(&pingWait),
		},
		{
			Flag:          "timeout",
			FlagShorthand: "t",
			Default:       "5s",
			Description:   "Specifies how long to wait for a ping to complete.",
			Value:         serpent.DurationOf(&pingTimeout),
		},
		{
			Flag:          "num",
			FlagShorthand: "n",
			Default:       "10",
			Description:   "Specifies the number of pings to perform.",
			Value:         serpent.Int64Of(&pingNum),
		},
	}
	return cmd
}
