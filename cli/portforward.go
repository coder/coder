package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pion/udp"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func portForward() *cobra.Command {
	var (
		tcpForwards []string // <port>:<port>
		udpForwards []string // <port>:<port>
	)
	cmd := &cobra.Command{
		Use:     "port-forward <workspace>",
		Short:   "Forward ports from machine to a workspace",
		Aliases: []string{"tunnel"},
		Args:    cobra.ExactArgs(1),
		Example: formatExamples(
			example{
				Description: "Port forward a single TCP port from 1234 in the workspace to port 5678 on your local machine",
				Command:     "coder port-forward <workspace> --tcp 5678:1234",
			},
			example{
				Description: "Port forward a single UDP port from port 9000 to port 9000 on your local machine",
				Command:     "coder port-forward <workspace> --udp 9000",
			},
			example{
				Description: "Port forward multiple TCP ports and a UDP port",
				Command:     "coder port-forward <workspace> --tcp 8080:8080 --tcp 9000:3000 --udp 5353:53",
			},
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			specs, err := parsePortForwards(tcpForwards, udpForwards)
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

			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			workspace, workspaceAgent, err := getWorkspaceAndAgent(ctx, cmd, client, codersdk.Me, args[0], false)
			if err != nil {
				return err
			}
			if workspace.LatestBuild.Transition != codersdk.WorkspaceTransitionStart {
				return xerrors.New("workspace must be in start transition to port-forward")
			}
			if workspace.LatestBuild.Job.CompletedAt == nil {
				err = cliui.WorkspaceBuild(ctx, cmd.ErrOrStderr(), client, workspace.LatestBuild.ID, workspace.CreatedAt)
				if err != nil {
					return err
				}
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

			conn, err := client.DialWorkspaceAgentTailnet(ctx, slog.Logger{}, workspaceAgent.ID)
			if err != nil {
				return err
			}
			defer conn.Close()

			// Start all listeners.
			var (
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
			defer closeAllListeners()

			for i, spec := range specs {
				l, err := listenAndPortForward(ctx, cmd, conn, wg, spec)
				if err != nil {
					return err
				}
				listeners[i] = l
			}

			// Wait for the context to be canceled or for a signal and close
			// all listeners.
			var closeErr error
			wg.Add(1)
			go func() {
				defer wg.Done()

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

			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
				}

				_, err = conn.Ping()
				if err != nil {
					continue
				}
				break
			}
			ticker.Stop()
			_, _ = fmt.Fprintln(cmd.OutOrStderr(), "Ready!")
			wg.Wait()
			return closeErr
		},
	}

	cmd.Flags().StringArrayVarP(&tcpForwards, "tcp", "p", []string{}, "Forward a TCP port from the workspace to the local machine")
	cmd.Flags().StringArrayVar(&udpForwards, "udp", []string{}, "Forward a UDP port from the workspace to the local machine. The UDP connection has TCP-like semantics to support stateful UDP protocols")
	return cmd
}

func listenAndPortForward(ctx context.Context, cmd *cobra.Command, conn *codersdk.AgentConn, wg *sync.WaitGroup, spec portForwardSpec) (net.Listener, error) {
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
				remoteConn, err := conn.DialContext(ctx, spec.dialNetwork, spec.dialAddress)
				if err != nil {
					_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Failed to dial '%v://%v' in workspace: %s\n", spec.dialNetwork, spec.dialAddress, err)
					return
				}
				defer remoteConn.Close()

				agent.Bicopy(ctx, netConn, remoteConn)
			}(netConn)
		}
	}(spec)

	return l, nil
}

type portForwardSpec struct {
	listenNetwork string // tcp, udp
	listenAddress string // <ip>:<port> or path

	dialNetwork string // tcp, udp
	dialAddress string // <ip>:<port> or path
}

func parsePortForwards(tcpSpecs, udpSpecs []string) ([]portForwardSpec, error) {
	specs := []portForwardSpec{}

	for _, spec := range tcpSpecs {
		local, remote, err := parsePortPort(spec)
		if err != nil {
			return nil, xerrors.Errorf("failed to parse TCP port-forward specification %q: %w", spec, err)
		}

		specs = append(specs, portForwardSpec{
			listenNetwork: "tcp",
			listenAddress: fmt.Sprintf("127.0.0.1:%v", local),
			dialNetwork:   "tcp",
			dialAddress:   fmt.Sprintf("127.0.0.1:%v", remote),
		})
	}

	for _, spec := range udpSpecs {
		local, remote, err := parsePortPort(spec)
		if err != nil {
			return nil, xerrors.Errorf("failed to parse UDP port-forward specification %q: %w", spec, err)
		}

		specs = append(specs, portForwardSpec{
			listenNetwork: "udp",
			listenAddress: fmt.Sprintf("127.0.0.1:%v", local),
			dialNetwork:   "udp",
			dialAddress:   fmt.Sprintf("127.0.0.1:%v", remote),
		})
	}

	// Check for duplicate entries.
	locals := map[string]struct{}{}
	for _, spec := range specs {
		localStr := fmt.Sprintf("%v:%v", spec.listenNetwork, spec.listenAddress)
		if _, ok := locals[localStr]; ok {
			return nil, xerrors.Errorf("local %v %v is specified twice", spec.listenNetwork, spec.listenAddress)
		}
		locals[localStr] = struct{}{}
	}

	return specs, nil
}

func parsePort(in string) (uint16, error) {
	port, err := strconv.ParseUint(strings.TrimSpace(in), 10, 16)
	if err != nil {
		return 0, xerrors.Errorf("parse port %q: %w", in, err)
	}
	if port == 0 {
		return 0, xerrors.New("port cannot be 0")
	}

	return uint16(port), nil
}

func parsePortPort(in string) (local uint16, remote uint16, err error) {
	parts := strings.Split(in, ":")
	if len(parts) > 2 {
		return 0, 0, xerrors.Errorf("invalid port specification %q", in)
	}
	if len(parts) == 1 {
		// Duplicate the single part
		parts = append(parts, parts[0])
	}

	local, err = parsePort(parts[0])
	if err != nil {
		return 0, 0, xerrors.Errorf("parse local port from %q: %w", in, err)
	}
	remote, err = parsePort(parts[1])
	if err != nil {
		return 0, 0, xerrors.Errorf("parse remote port from %q: %w", in, err)
	}

	return local, remote, nil
}
