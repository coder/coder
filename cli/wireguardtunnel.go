package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"github.com/google/uuid"
	"github.com/pion/udp"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
	"inet.af/netaddr"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	coderagent "github.com/coder/coder/agent"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/peer/peerwg"
)

func wireguardPortForward() *cobra.Command {
	var (
		tcpForwards []string // <port>:<port>
		udpForwards []string // <port>:<port>
		// TODO: unix support
		// unixForwards []string // <path>:<path> OR <port>:<path>
	)
	cmd := &cobra.Command{
		Use:     "wireguard-port-forward <workspace>",
		Aliases: []string{"wireguard-tunnel"},
		Args:    cobra.ExactArgs(1),
		// Hide all wireguard commands for now while we test!
		Hidden: true,
		Example: formatExamples(
			example{
				Description: "Port forward a single TCP port from 1234 in the workspace to port 5678 on your local machine",
				Command:     "coder wireguard-port-forward <workspace> --tcp 5678:1234",
			},
			example{
				Description: "Port forward a single UDP port from port 9000 to port 9000 on your local machine",
				Command:     "coder wireguard-port-forward <workspace> --udp 9000",
			},
			example{
				Description: "Port forward multiple TCP ports and a UDP port",
				Command:     "coder wireguard-port-forward <workspace> --tcp 8080:8080 --tcp 9000:3000 --udp 5353:53",
			},
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			specs, err := parsePortForwards(tcpForwards, nil, nil)
			if err != nil {
				return xerrors.Errorf("parse port-forward specs: %w", err)
			}
			if len(specs) == 0 {
				err = cmd.Help()
				if err != nil {
					return xerrors.Errorf("generate help output: %w", err)
				}
				return xerrors.New("no port-forwards requested")
			}

			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			workspace, workspaceAgent, err := getWorkspaceAndAgent(cmd, client, codersdk.Me, args[0], false)
			if err != nil {
				return err
			}
			if workspace.LatestBuild.Transition != codersdk.WorkspaceTransitionStart {
				return xerrors.New("workspace must be in start transition to port-forward")
			}
			if workspace.LatestBuild.Job.CompletedAt == nil {
				err = cliui.WorkspaceBuild(cmd.Context(), cmd.ErrOrStderr(), client, workspace.LatestBuild.ID, workspace.CreatedAt)
				if err != nil {
					return err
				}
			}

			err = cliui.Agent(cmd.Context(), cmd.ErrOrStderr(), cliui.AgentOptions{
				WorkspaceName: workspace.Name,
				Fetch: func(ctx context.Context) (codersdk.WorkspaceAgent, error) {
					return client.WorkspaceAgent(ctx, workspaceAgent.ID)
				},
			})
			if err != nil {
				return xerrors.Errorf("await agent: %w", err)
			}

			ipv6 := peerwg.UUIDToNetaddr(uuid.New())
			wgn, err := peerwg.New(
				slog.Make(sloghuman.Sink(cmd.ErrOrStderr())),
				[]netaddr.IPPrefix{netaddr.IPPrefixFrom(ipv6, 128)},
			)
			if err != nil {
				return xerrors.Errorf("create wireguard network: %w", err)
			}

			err = client.PostWireguardPeer(cmd.Context(), workspace.ID, peerwg.Handshake{
				Recipient:      workspaceAgent.ID,
				NodePublicKey:  wgn.NodePrivateKey.Public(),
				DiscoPublicKey: wgn.DiscoPublicKey,
				IPv6:           ipv6,
			})
			if err != nil {
				return xerrors.Errorf("post wireguard peer: %w", err)
			}

			err = wgn.AddPeer(peerwg.Handshake{
				Recipient:      workspaceAgent.ID,
				DiscoPublicKey: workspaceAgent.DiscoPublicKey,
				NodePublicKey:  workspaceAgent.WireguardPublicKey,
				IPv6:           workspaceAgent.IPv6.IP(),
			})
			if err != nil {
				return xerrors.Errorf("add workspace agent as peer: %w", err)
			}

			// Start all listeners.
			var (
				ctx, cancel       = context.WithCancel(cmd.Context())
				wg                = new(sync.WaitGroup)
				listeners         = make([]net.Listener, len(specs))
				closeAllListeners = func() {
					for _, l := range listeners {
						if l == nil {
							continue
						}
						_ = l.Close()
					}
				}
			)
			defer cancel()
			for i, spec := range specs {
				l, err := listenAndPortForwardWireguard(ctx, cmd, wgn, wg, spec, workspaceAgent.IPv6.IP())
				if err != nil {
					closeAllListeners()
					return err
				}
				listeners[i] = l
			}

			// Wait for the context to be canceled or for a signal and close
			// all listeners.
			var closeErr error
			go func() {
				sigs := make(chan os.Signal, 1)
				signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

				select {
				case <-ctx.Done():
					closeErr = ctx.Err()
				case <-sigs:
					_, _ = fmt.Fprintln(cmd.OutOrStderr(), "Received signal, closing all listeners and active connections")
					closeErr = xerrors.New("signal received")
				}

				cancel()
				closeAllListeners()
			}()

			_, _ = fmt.Fprintln(cmd.OutOrStderr(), "Ready!")
			wg.Wait()
			return closeErr
		},
	}

	cmd.Flags().StringArrayVarP(&tcpForwards, "tcp", "p", []string{}, "Forward a TCP port from the workspace to the local machine")
	cmd.Flags().StringArrayVar(&udpForwards, "udp", []string{}, "Forward a UDP port from the workspace to the local machine. The UDP connection has TCP-like semantics to support stateful UDP protocols")
	// cmd.Flags().StringArrayVar(&unixForwards, "unix", []string{}, "Forward a Unix socket in the workspace to a local Unix socket or TCP port")

	return cmd
}

func listenAndPortForwardWireguard(ctx context.Context, cmd *cobra.Command,
	wgn *peerwg.Network,
	wg *sync.WaitGroup,
	spec portForwardSpec,
	agentIP netaddr.IP,
) (net.Listener, error) {
	_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Forwarding '%v://%v' locally to '%v://%v' in the workspace\n", spec.listenNetwork, spec.listenAddress, spec.dialNetwork, spec.dialAddress)

	var (
		l   net.Listener
		err error
	)
	switch spec.listenNetwork {
	case "tcp":
		l, err = net.Listen(spec.listenNetwork, spec.listenAddress)
	case "udp":
		var host, port string
		host, port, err = net.SplitHostPort(spec.listenAddress)
		if err != nil {
			return nil, xerrors.Errorf("split %q: %w", spec.listenAddress, err)
		}

		var portInt int
		portInt, err = strconv.Atoi(port)
		if err != nil {
			return nil, xerrors.Errorf("parse port %v from %q as int: %w", port, spec.listenAddress, err)
		}

		l, err = udp.Listen(spec.listenNetwork, &net.UDPAddr{
			IP:   net.ParseIP(host),
			Port: portInt,
		})
	// case "unix":
	// 	l, err = net.Listen(spec.listenNetwork, spec.listenAddress)
	default:
		return nil, xerrors.Errorf("unknown listen network %q", spec.listenNetwork)
	}
	if err != nil {
		return nil, xerrors.Errorf("listen '%v://%v': %w", spec.listenNetwork, spec.listenAddress, err)
	}

	wg.Add(1)
	go func(spec portForwardSpec) {
		defer wg.Done()
		for {
			netConn, err := l.Accept()
			if err != nil {
				_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Error accepting connection from '%v://%v': %+v\n", spec.listenNetwork, spec.listenAddress, err)
				_, _ = fmt.Fprintln(cmd.OutOrStderr(), "Killing listener")
				return
			}

			go func(netConn net.Conn) {
				defer netConn.Close()

				ipPort := netaddr.MustParseIPPort(spec.dialAddress).WithIP(agentIP)

				var remoteConn net.Conn
				switch spec.dialNetwork {
				case "tcp":
					remoteConn, err = wgn.Netstack.DialContextTCP(ctx, ipPort)
				case "udp":
					remoteConn, err = wgn.Netstack.DialContextUDP(ctx, ipPort)
				}
				if err != nil {
					_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Failed to dial '%v://%v' in workspace: %s\n", spec.dialNetwork, spec.dialAddress, err)
					return
				}
				defer remoteConn.Close()

				coderagent.Bicopy(ctx, netConn, remoteConn)
			}(netConn)
		}
	}(spec)

	return l, nil
}
