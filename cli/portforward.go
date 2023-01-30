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

	"github.com/pion/udp"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/cliflag"
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
			example{
				Description: "Port forward multiple ports (TCP or UDP) in condensed syntax",
				Command:     "coder port-forward <workspace> --tcp 8080,9000:3000,9090-9092,10000-10002:10010-10012",
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
				err = cliui.WorkspaceBuild(ctx, cmd.ErrOrStderr(), client, workspace.LatestBuild.ID)
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

			conn, err := client.DialWorkspaceAgent(ctx, workspaceAgent.ID, nil)
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
					_, _ = fmt.Fprintln(cmd.OutOrStderr(), "\nReceived signal, closing all listeners and active connections")
				}

				cancel()
				closeAllListeners()
			}()

			conn.AwaitReachable(ctx)
			_, _ = fmt.Fprintln(cmd.OutOrStderr(), "Ready!")
			wg.Wait()
			return closeErr
		},
	}

	cliflag.StringArrayVarP(cmd.Flags(), &tcpForwards, "tcp", "p", "CODER_PORT_FORWARD_TCP", nil, "Forward TCP port(s) from the workspace to the local machine")
	cliflag.StringArrayVarP(cmd.Flags(), &udpForwards, "udp", "", "CODER_PORT_FORWARD_UDP", nil, "Forward UDP port(s) from the workspace to the local machine. The UDP connection has TCP-like semantics to support stateful UDP protocols")
	return cmd
}

func listenAndPortForward(ctx context.Context, cmd *cobra.Command, conn *codersdk.WorkspaceAgentConn, wg *sync.WaitGroup, spec portForwardSpec) (net.Listener, error) {
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
				// Silently ignore net.ErrClosed errors.
				if xerrors.Is(err, net.ErrClosed) {
					return
				}
				_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Error accepting connection from '%v://%v': %v\n", spec.listenNetwork, spec.listenAddress, err)
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

	for _, specEntry := range tcpSpecs {
		for _, spec := range strings.Split(specEntry, ",") {
			ports, err := parseSrcDestPorts(spec)
			if err != nil {
				return nil, xerrors.Errorf("failed to parse TCP port-forward specification %q: %w", spec, err)
			}

			for _, port := range ports {
				specs = append(specs, portForwardSpec{
					listenNetwork: "tcp",
					listenAddress: fmt.Sprintf("127.0.0.1:%v", port.local),
					dialNetwork:   "tcp",
					dialAddress:   fmt.Sprintf("127.0.0.1:%v", port.remote),
				})
			}
		}
	}

	for _, specEntry := range udpSpecs {
		for _, spec := range strings.Split(specEntry, ",") {
			ports, err := parseSrcDestPorts(spec)
			if err != nil {
				return nil, xerrors.Errorf("failed to parse UDP port-forward specification %q: %w", spec, err)
			}

			for _, port := range ports {
				specs = append(specs, portForwardSpec{
					listenNetwork: "udp",
					listenAddress: fmt.Sprintf("127.0.0.1:%v", port.local),
					dialNetwork:   "udp",
					dialAddress:   fmt.Sprintf("127.0.0.1:%v", port.remote),
				})
			}
		}
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

type parsedSrcDestPort struct {
	local, remote uint16
}

func parseSrcDestPorts(in string) ([]parsedSrcDestPort, error) {
	parts := strings.Split(in, ":")
	if len(parts) > 2 {
		return nil, xerrors.Errorf("invalid port specification %q", in)
	}
	if len(parts) == 1 {
		// Duplicate the single part
		parts = append(parts, parts[0])
	}
	if !strings.Contains(parts[0], "-") {
		local, err := parsePort(parts[0])
		if err != nil {
			return nil, xerrors.Errorf("parse local port from %q: %w", in, err)
		}
		remote, err := parsePort(parts[1])
		if err != nil {
			return nil, xerrors.Errorf("parse remote port from %q: %w", in, err)
		}

		return []parsedSrcDestPort{{local: local, remote: remote}}, nil
	}

	local, err := parsePortRange(parts[0])
	if err != nil {
		return nil, xerrors.Errorf("parse local port range from %q: %w", in, err)
	}
	remote, err := parsePortRange(parts[1])
	if err != nil {
		return nil, xerrors.Errorf("parse remote port range from %q: %w", in, err)
	}
	if len(local) != len(remote) {
		return nil, xerrors.Errorf("port ranges must be the same length, got %d ports forwarded to %d ports", len(local), len(remote))
	}
	var out []parsedSrcDestPort
	for i := range local {
		out = append(out, parsedSrcDestPort{
			local:  local[i],
			remote: remote[i],
		})
	}
	return out, nil
}

func parsePortRange(in string) ([]uint16, error) {
	parts := strings.Split(in, "-")
	if len(parts) != 2 {
		return nil, xerrors.Errorf("invalid port range specification %q", in)
	}
	start, err := parsePort(parts[0])
	if err != nil {
		return nil, xerrors.Errorf("parse range start port from %q: %w", in, err)
	}
	end, err := parsePort(parts[1])
	if err != nil {
		return nil, xerrors.Errorf("parse range end port from %q: %w", in, err)
	}
	if end < start {
		return nil, xerrors.Errorf("range end port %v is less than start port %v", end, start)
	}
	var ports []uint16
	for i := start; i <= end; i++ {
		ports = append(ports, i)
	}
	return ports, nil
}
