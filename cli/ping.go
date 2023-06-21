package cli

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) ping() *clibase.Cmd {
	var (
		pingNum     int64
		pingTimeout time.Duration
		pingWait    time.Duration
	)

	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "ping <workspace>",
		Short:       "Ping a workspace",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			workspaceName := inv.Args[0]
			_, workspaceAgent, err := getWorkspaceAndAgent(
				ctx, inv, client,
				codersdk.Me, workspaceName,
			)
			if err != nil {
				return err
			}

			var logger slog.Logger
			if r.verbose {
				logger = slog.Make(sloghuman.Sink(inv.Stdout)).Leveled(slog.LevelDebug)
			}

			if r.disableDirect {
				_, _ = fmt.Fprintln(inv.Stderr, "Direct connections disabled.")
			}
			conn, err := client.DialWorkspaceAgent(ctx, workspaceAgent.ID, &codersdk.DialWorkspaceAgentOptions{
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
					time.Sleep(time.Second)
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
							cliui.DefaultStyles.DateTimeStamp.Render(time.Since(start).Round(time.Millisecond).String()),
						)
					}
					didP2p = true

					via = fmt.Sprintf("%s via %s",
						cliui.DefaultStyles.Fuchsia.Render("p2p"),
						cliui.DefaultStyles.Code.Render(pong.Endpoint),
					)
				} else {
					derpName := "unknown"
					derpRegion, ok := derpMap.Regions[pong.DERPRegionID]
					if ok {
						derpName = derpRegion.RegionName
					}
					via = fmt.Sprintf("%s via %s",
						cliui.DefaultStyles.Fuchsia.Render("proxied"),
						cliui.DefaultStyles.Code.Render(fmt.Sprintf("DERP(%s)", derpName)),
					)
				}

				_, _ = fmt.Fprintf(inv.Stdout, "pong from %s %s in %s\n",
					cliui.DefaultStyles.Keyword.Render(workspaceName),
					via,
					cliui.DefaultStyles.DateTimeStamp.Render(dur.String()),
				)

				if n == int(pingNum) {
					return nil
				}
			}
		},
	}

	cmd.Options = clibase.OptionSet{
		{
			Flag:        "wait",
			Description: "Specifies how long to wait between pings.",
			Default:     "1s",
			Value:       clibase.DurationOf(&pingWait),
		},
		{
			Flag:          "timeout",
			FlagShorthand: "t",
			Default:       "5s",
			Description:   "Specifies how long to wait for a ping to complete.",
			Value:         clibase.DurationOf(&pingTimeout),
		},
		{
			Flag:          "num",
			FlagShorthand: "n",
			Default:       "10",
			Description:   "Specifies the number of pings to perform.",
			Value:         clibase.Int64Of(&pingNum),
		},
	}
	return cmd
}
