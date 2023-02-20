package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func ping() *cobra.Command {
	var (
		pingNum     int
		pingTimeout time.Duration
		pingWait    time.Duration
		verbose     bool
	)
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "ping <workspace>",
		Short:       "Ping a workspace",
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			workspaceName := args[0]
			_, workspaceAgent, err := getWorkspaceAndAgent(ctx, cmd, client, codersdk.Me, workspaceName, false)
			if err != nil {
				return err
			}

			var logger slog.Logger
			if verbose {
				logger = slog.Make(sloghuman.Sink(cmd.OutOrStdout())).Leveled(slog.LevelDebug)
			}

			conn, err := client.DialWorkspaceAgent(ctx, workspaceAgent.ID, &codersdk.DialWorkspaceAgentOptions{Logger: logger})
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
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ping to %q timed out \n", workspaceName)
						if n == pingNum {
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

					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ping to %q failed %s\n", workspaceName, err.Error())
					if n == pingNum {
						return nil
					}
					continue
				}

				dur = dur.Round(time.Millisecond)
				var via string
				if p2p {
					if !didP2p {
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), "p2p connection established in",
							cliui.Styles.DateTimeStamp.Render(time.Since(start).Round(time.Millisecond).String()),
						)
					}
					didP2p = true

					via = fmt.Sprintf("%s via %s",
						cliui.Styles.Fuchsia.Render("p2p"),
						cliui.Styles.Code.Render(pong.Endpoint),
					)
				} else {
					derpName := "unknown"
					derpRegion, ok := derpMap.Regions[pong.DERPRegionID]
					if ok {
						derpName = derpRegion.RegionName
					}
					via = fmt.Sprintf("%s via %s",
						cliui.Styles.Fuchsia.Render("proxied"),
						cliui.Styles.Code.Render(fmt.Sprintf("DERP(%s)", derpName)),
					)
				}

				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "pong from %s %s in %s\n",
					cliui.Styles.Keyword.Render(workspaceName),
					via,
					cliui.Styles.DateTimeStamp.Render(dur.String()),
				)

				if n == pingNum {
					return nil
				}
			}
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enables verbose logging.")
	cmd.Flags().DurationVarP(&pingWait, "wait", "", time.Second, "Specifies how long to wait between pings.")
	cmd.Flags().DurationVarP(&pingTimeout, "timeout", "t", 5*time.Second, "Specifies how long to wait for a ping to complete.")
	cmd.Flags().IntVarP(&pingNum, "num", "n", 10, "Specifies the number of pings to perform.")
	return cmd
}
