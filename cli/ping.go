package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"golang.org/x/xerrors"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/briandowns/spinner"

	"github.com/coder/pretty"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/healthsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type pingSummary struct {
	Workspace  string         `table:"workspace,nosort"`
	Total      int            `table:"total"`
	Successful int            `table:"successful"`
	Min        *time.Duration `table:"min"`
	Avg        *time.Duration `table:"avg"`
	Max        *time.Duration `table:"max"`
	Variance   *time.Duration `table:"variance"`
	latencySum float64
	runningAvg float64
	m2         float64
}

func (s *pingSummary) addResult(r *ipnstate.PingResult) {
	s.Total++
	if r == nil || r.Err != "" {
		return
	}
	s.Successful++
	if s.Min == nil || r.LatencySeconds < s.Min.Seconds() {
		s.Min = ptr.Ref(time.Duration(r.LatencySeconds * float64(time.Second)))
	}
	if s.Max == nil || r.LatencySeconds > s.Min.Seconds() {
		s.Max = ptr.Ref(time.Duration(r.LatencySeconds * float64(time.Second)))
	}
	s.latencySum += r.LatencySeconds

	d := r.LatencySeconds - s.runningAvg
	s.runningAvg += d / float64(s.Successful)
	d2 := r.LatencySeconds - s.runningAvg
	s.m2 += d * d2
}

// Write finalizes the summary and writes it
func (s *pingSummary) Write(w io.Writer) {
	if s.Successful > 0 {
		s.Avg = ptr.Ref(time.Duration(s.latencySum / float64(s.Successful) * float64(time.Second)))
	}
	if s.Successful > 1 {
		s.Variance = ptr.Ref(time.Duration((s.m2 / float64(s.Successful-1)) * float64(time.Second)))
	}
	out, err := cliui.DisplayTable([]*pingSummary{s}, "", nil)
	if err != nil {
		_, _ = fmt.Fprintf(w, "Failed to display ping summary: %v\n", err)
		return
	}
	width := len(strings.Split(out, "\n")[0])
	_, _ = fmt.Println(strings.Repeat("-", width))
	_, _ = fmt.Fprint(w, out)
}

func (r *RootCmd) ping() *serpent.Command {
	var (
		pingNum          int64
		pingTimeout      time.Duration
		pingWait         time.Duration
		pingTimeLocal    bool
		pingTimeUTC      bool
		appearanceConfig codersdk.AppearanceConfig
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "ping <workspace>",
		Short:       "Ping a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
			initAppearance(client, &appearanceConfig),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			notifyCtx, notifyCancel := inv.SignalNotifyContext(ctx, StopSignals...)
			defer notifyCancel()

			workspaceName := inv.Args[0]
			_, workspaceAgent, err := getWorkspaceAndAgent(
				ctx, inv, client,
				false, // Do not autostart for a ping.
				workspaceName,
			)
			if err != nil {
				return err
			}

			// Start spinner after any build logs have finished streaming
			spin := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
			spin.Writer = inv.Stderr
			spin.Suffix = pretty.Sprint(cliui.DefaultStyles.Keyword, " Collecting diagnostics...")
			if !r.verbose {
				spin.Start()
			}

			opts := &workspacesdk.DialAgentOptions{}

			if r.verbose {
				opts.Logger = inv.Logger.AppendSinks(sloghuman.Sink(inv.Stdout)).Leveled(slog.LevelDebug)
			}

			if r.disableDirect {
				opts.BlockEndpoints = true
			}
			if !r.disableNetworkTelemetry {
				opts.EnableTelemetry = true
			}
			wsClient := workspacesdk.New(client)
			conn, err := wsClient.DialAgent(ctx, workspaceAgent.ID, opts)
			if err != nil {
				spin.Stop()
				return err
			}
			defer conn.Close()

			derpMap := conn.DERPMap()

			diagCtx, diagCancel := context.WithTimeout(inv.Context(), 30*time.Second)
			defer diagCancel()
			diags := conn.GetPeerDiagnostics()

			// Silent ping to determine whether we should show diags
			_, didP2p, _, _ := conn.Ping(ctx)

			ni := conn.GetNetInfo()
			connDiags := cliui.ConnDiags{
				DisableDirect:      r.disableDirect,
				LocalNetInfo:       ni,
				Verbose:            r.verbose,
				PingP2P:            didP2p,
				TroubleshootingURL: appearanceConfig.DocsURL + "/admin/networking/troubleshooting",
			}

			awsRanges, err := cliutil.FetchAWSIPRanges(diagCtx, cliutil.AWSIPRangesURL)
			if err != nil {
				opts.Logger.Debug(inv.Context(), "failed to retrieve AWS IP ranges", slog.Error(err))
			}

			connDiags.ClientIPIsAWS = isAWSIP(awsRanges, ni)

			connInfo, err := wsClient.AgentConnectionInfoGeneric(diagCtx)
			if err != nil || connInfo.DERPMap == nil {
				spin.Stop()
				return xerrors.Errorf("Failed to retrieve connection info from server: %w\n", err)
			}
			connDiags.ConnInfo = connInfo
			ifReport, err := healthsdk.RunInterfacesReport()
			if err == nil {
				connDiags.LocalInterfaces = &ifReport
			} else {
				_, _ = fmt.Fprintf(inv.Stdout, "Failed to retrieve local interfaces report: %v\n", err)
			}

			agentNetcheck, err := conn.Netcheck(diagCtx)
			if err == nil {
				connDiags.AgentNetcheck = &agentNetcheck
				connDiags.AgentIPIsAWS = isAWSIP(awsRanges, agentNetcheck.NetInfo)
			} else {
				var sdkErr *codersdk.Error
				if errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound {
					_, _ = fmt.Fprint(inv.Stdout, "Could not generate full connection report as the workspace agent is outdated\n")
				} else {
					_, _ = fmt.Fprintf(inv.Stdout, "Failed to retrieve connection report from agent: %v\n", err)
				}
			}

			spin.Stop()
			cliui.PeerDiagnostics(inv.Stderr, diags)
			connDiags.Write(inv.Stderr)
			results := &pingSummary{
				Workspace: workspaceName,
			}
			var (
				pong *ipnstate.PingResult
				dur  time.Duration
				p2p  bool
			)
			n := 0
			start := time.Now()
		pingLoop:
			for {
				if n > 0 {
					time.Sleep(pingWait)
				}
				n++

				ctx, cancel := context.WithTimeout(ctx, pingTimeout)
				dur, p2p, pong, err = conn.Ping(ctx)
				pongTime := time.Now()
				if pingTimeUTC {
					pongTime = pongTime.UTC()
				}
				cancel()
				results.addResult(pong)
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

				var displayTime string
				if pingTimeLocal || pingTimeUTC {
					displayTime = pretty.Sprintf(cliui.DefaultStyles.DateTimeStamp, "[%s] ", pongTime.Format(time.RFC3339))
				}

				_, _ = fmt.Fprintf(inv.Stdout, "%spong from %s %s in %s\n",
					displayTime,
					pretty.Sprint(cliui.DefaultStyles.Keyword, workspaceName),
					via,
					pretty.Sprint(cliui.DefaultStyles.DateTimeStamp, dur.String()),
				)

				select {
				case <-notifyCtx.Done():
					break pingLoop
				default:
					if n == int(pingNum) {
						break pingLoop
					}
				}
			}

			if p2p {
				msg := "✔ You are connected directly (p2p)"
				if pong != nil && isPrivateEndpoint(pong.Endpoint) {
					msg += ", over a private network"
				}
				_, _ = fmt.Fprintln(inv.Stderr, msg)
			} else {
				_, _ = fmt.Fprintf(inv.Stderr, "❗ You are connected via a DERP relay, not directly (p2p)\n"+
					"   %s#common-problems-with-direct-connections\n", connDiags.TroubleshootingURL)
			}

			results.Write(inv.Stdout)

			return nil
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
			Description:   "Specifies the number of pings to perform. By default, pings will continue until interrupted.",
			Value:         serpent.Int64Of(&pingNum),
		},
		{
			Flag:        "time",
			Description: "Show the response time of each pong in local time.",
			Value:       serpent.BoolOf(&pingTimeLocal),
		},
		{
			Flag:        "utc",
			Description: "Show the response time of each pong in UTC (implies --time).",
			Value:       serpent.BoolOf(&pingTimeUTC),
		},
	}
	return cmd
}

func isAWSIP(awsRanges *cliutil.AWSIPRanges, ni *tailcfg.NetInfo) bool {
	if awsRanges == nil {
		return false
	}
	if ni.GlobalV4 != "" {
		ip, err := netip.ParseAddr(ni.GlobalV4)
		if err == nil && awsRanges.CheckIP(ip) {
			return true
		}
	}
	if ni.GlobalV6 != "" {
		ip, err := netip.ParseAddr(ni.GlobalV6)
		if err == nil && awsRanges.CheckIP(ip) {
			return true
		}
	}
	return false
}

func isPrivateEndpoint(endpoint string) bool {
	ip, err := netip.ParseAddrPort(endpoint)
	if err != nil {
		return false
	}
	return ip.Addr().IsPrivate()
}
