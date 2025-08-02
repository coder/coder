package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/portforward"
	"github.com/coder/serpent"
)

// cliDialer adapts workspacesdk.AgentConn to portforward.Dialer
type cliDialer struct {
	conn *workspacesdk.AgentConn
}

func (d *cliDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return d.conn.DialContext(ctx, network, address)
}

// cliListener adapts serpent.Invocation.Net to portforward.Listener
type cliListener struct {
	inv *serpent.Invocation
}

func (l *cliListener) Listen(network, address string) (net.Listener, error) {
	return l.inv.Net.Listen(network, address)
}

func (r *RootCmd) portForward() *serpent.Command {
	var (
		tcpForwards      []string // <port>:<port>
		udpForwards      []string // <port>:<port>
		disableAutostart bool
		appearanceConfig codersdk.AppearanceConfig
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:     "port-forward <workspace>",
		Short:   `Forward ports from a workspace to the local machine. For reverse port forwarding, use "coder ssh -R".`,
		Aliases: []string{"tunnel"},
		Long: FormatExamples(
			Example{
				Description: "Port forward a single TCP port from 1234 in the workspace to port 5678 on your local machine",
				Command:     "coder port-forward <workspace> --tcp 5678:1234",
			},
			Example{
				Description: "Port forward a single UDP port from port 9000 to port 9000 on your local machine",
				Command:     "coder port-forward <workspace> --udp 9000",
			},
			Example{
				Description: "Port forward multiple TCP ports and a UDP port",
				Command:     "coder port-forward <workspace> --tcp 8080:8080 --tcp 9000:3000 --udp 5353:53",
			},
			Example{
				Description: "Port forward multiple ports (TCP or UDP) in condensed syntax",
				Command:     "coder port-forward <workspace> --tcp 8080,9000:3000,9090-9092,10000-10002:10010-10012",
			},
			Example{
				Description: "Port forward specifying the local address to bind to",
				Command:     "coder port-forward <workspace> --tcp 1.2.3.4:8080:8080",
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
			initAppearance(client, &appearanceConfig),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			specs, err := portforward.ParseSpecs(tcpForwards, udpForwards)
			if err != nil {
				return xerrors.Errorf("parse port-forward specs: %w", err)
			}
			if len(specs) == 0 {
				return xerrors.New("no port-forwards requested")
			}

			workspace, workspaceAgent, _, err := getWorkspaceAndAgent(ctx, inv, client, !disableAutostart, inv.Args[0])
			if err != nil {
				return err
			}
			if workspace.LatestBuild.Transition != codersdk.WorkspaceTransitionStart {
				return xerrors.New("workspace must be in start transition to port-forward")
			}
			if workspace.LatestBuild.Job.CompletedAt == nil {
				err = cliui.WorkspaceBuild(ctx, inv.Stderr, client, workspace.LatestBuild.ID)
				if err != nil {
					return err
				}
			}

			err = cliui.Agent(ctx, inv.Stderr, workspaceAgent.ID, cliui.AgentOptions{
				Fetch:   client.WorkspaceAgent,
				Wait:    false,
				DocsURL: appearanceConfig.DocsURL,
			})
			if err != nil {
				return xerrors.Errorf("await agent: %w", err)
			}

			opts := &workspacesdk.DialAgentOptions{}

			logger := inv.Logger
			if r.verbose {
				opts.Logger = logger.AppendSinks(sloghuman.Sink(inv.Stdout)).Leveled(slog.LevelDebug)
			}

			if r.disableDirect {
				_, _ = fmt.Fprintln(inv.Stderr, "Direct connections disabled.")
				opts.BlockEndpoints = true
			}
			if !r.disableNetworkTelemetry {
				opts.EnableTelemetry = true
			}
			conn, err := workspacesdk.New(client).DialAgent(ctx, workspaceAgent.ID, opts)
			if err != nil {
				return err
			}
			defer conn.Close()

			// Create port forwarding options
			pfOpts := portforward.Options{
				Logger:   logger,
				Dialer:   &cliDialer{conn: conn},
				Listener: &cliListener{inv: inv},
			}

			// Start all forwarders.
			var (
				forwarders         = make([]portforward.Forwarder, 0, len(specs))
				closeAllForwarders = func() {
					logger.Debug(ctx, "closing all forwarders")
					for _, f := range forwarders {
						_ = f.Stop()
					}
				}
			)
			defer closeAllForwarders()

			// Create a signal handler for graceful shutdown
			shutdownCh := make(chan struct{})
			go func() {
				defer close(shutdownCh)

				// Wait until context is canceled (Ctrl+C, etc.)
				<-ctx.Done()
			}()

			for _, spec := range specs {
				if spec.ListenHost == portforward.NoAddr {
					// first, opportunistically try to listen on IPv6
					spec6 := spec
					spec6.ListenHost = portforward.IPv6Loopback
					f6 := portforward.NewForwarder(spec6, pfOpts)
					err6 := f6.Start(ctx)
					if err6 != nil {
						logger.Info(ctx, "failed to opportunistically listen on IPv6", slog.F("spec", spec), slog.Error(err6))
					} else {
						forwarders = append(forwarders, f6)
						_, _ = fmt.Fprintf(inv.Stderr, "Forwarding '%s://[%s]:%d' locally to '%s://127.0.0.1:%d' in the workspace\n",
							spec6.Network, spec6.ListenHost, spec6.ListenPort, spec6.Network, spec6.DialPort)
					}
					spec.ListenHost = portforward.IPv4Loopback
				}

				f := portforward.NewForwarder(spec, pfOpts)
				err := f.Start(ctx)
				if err != nil {
					logger.Error(ctx, "failed to listen", slog.F("spec", spec), slog.Error(err))
					return err
				}

				forwarders = append(forwarders, f)
				_, _ = fmt.Fprintf(inv.Stderr, "Forwarding '%s://%s:%d' locally to '%s://127.0.0.1:%d' in the workspace\n",
					spec.Network, spec.ListenHost, spec.ListenPort, spec.Network, spec.DialPort)
			}

			conn.AwaitReachable(ctx)
			logger.Debug(ctx, "ready to accept connections to forward")
			_, _ = fmt.Fprintln(inv.Stderr, "Ready!")

			stopUpdating := client.UpdateWorkspaceUsageContext(ctx, workspace.ID)
			defer stopUpdating()

			// Wait for shutdown signal or context cancellation
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

			select {
			case <-shutdownCh:
				logger.Debug(ctx, "context canceled")
				return ctx.Err()
			case sig := <-sigs:
				logger.Debug(ctx, "received signal", slog.F("signal", sig))
				_, _ = fmt.Fprintln(inv.Stderr, "\nReceived signal, closing all listeners and active connections")
				return nil
			}
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:          "tcp",
			FlagShorthand: "p",
			Env:           "CODER_PORT_FORWARD_TCP",
			Description:   "Forward TCP port(s) from the workspace to the local machine.",
			Value:         serpent.StringArrayOf(&tcpForwards),
		},
		{
			Flag:        "udp",
			Env:         "CODER_PORT_FORWARD_UDP",
			Description: "Forward UDP port(s) from the workspace to the local machine. The UDP connection has TCP-like semantics to support stateful UDP protocols.",
			Value:       serpent.StringArrayOf(&udpForwards),
		},
		sshDisableAutostartOption(serpent.BoolOf(&disableAutostart)),
	}

	return cmd
}
