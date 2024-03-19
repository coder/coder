package cli

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) portForward() *serpent.Command {
	var (
		tcpForwards      []string // <port>:<port>
		udpForwards      []string // <port>:<port>
		disableAutostart bool
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:     "port-forward <workspace>",
		Short:   `Forward ports from a workspace to the local machine. For reverse port forwarding, use "coder ssh -R".`,
		Aliases: []string{"tunnel"},
		Long: formatExamples(
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
			example{
				Description: "Port forward specifying the local address to bind to",
				Command:     "coder port-forward <workspace> --tcp 1.2.3.4:8080:8080",
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			specs, err := parsePortForwards(tcpForwards, udpForwards)
			if err != nil {
				return xerrors.Errorf("parse port-forward specs: %w", err)
			}
			if len(specs) == 0 {
				return xerrors.New("no port-forwards requested")
			}

			workspace, workspaceAgent, err := getWorkspaceAndAgent(ctx, inv, client, !disableAutostart, codersdk.Me, inv.Args[0])
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
				Fetch: client.WorkspaceAgent,
				Wait:  false,
			})
			if err != nil {
				return xerrors.Errorf("await agent: %w", err)
			}

			logger := inv.Logger
			if r.verbose {
				logger = logger.AppendSinks(sloghuman.Sink(inv.Stdout)).Leveled(slog.LevelDebug)
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

			// Start all listeners.
			var (
				wg                = new(sync.WaitGroup)
				listeners         = make([]net.Listener, len(specs))
				closeAllListeners = func() {
					logger.Debug(ctx, "closing all listeners")
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
				l, err := listenAndPortForward(ctx, inv, conn, wg, spec, logger)
				if err != nil {
					logger.Error(ctx, "failed to listen", slog.F("spec", spec), slog.Error(err))
					return err
				}
				listeners[i] = l
			}

			stopUpdating := client.UpdateWorkspaceUsageContext(ctx, workspace.ID)

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
					logger.Debug(ctx, "command context expired waiting for signal", slog.Error(ctx.Err()))
					closeErr = ctx.Err()
				case sig := <-sigs:
					logger.Debug(ctx, "received signal", slog.F("signal", sig))
					_, _ = fmt.Fprintln(inv.Stderr, "\nReceived signal, closing all listeners and active connections")
				}

				cancel()
				stopUpdating()
				closeAllListeners()
			}()

			conn.AwaitReachable(ctx)
			logger.Debug(ctx, "read to accept connections to forward")
			_, _ = fmt.Fprintln(inv.Stderr, "Ready!")
			wg.Wait()
			return closeErr
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

func listenAndPortForward(
	ctx context.Context,
	inv *serpent.Invocation,
	conn *codersdk.WorkspaceAgentConn,
	wg *sync.WaitGroup,
	spec portForwardSpec,
	logger slog.Logger,
) (net.Listener, error) {
	logger = logger.With(slog.F("network", spec.listenNetwork), slog.F("address", spec.listenAddress))
	_, _ = fmt.Fprintf(inv.Stderr, "Forwarding '%v://%v' locally to '%v://%v' in the workspace\n", spec.listenNetwork, spec.listenAddress, spec.dialNetwork, spec.dialAddress)

	l, err := inv.Net.Listen(spec.listenNetwork, spec.listenAddress)
	if err != nil {
		return nil, xerrors.Errorf("listen '%v://%v': %w", spec.listenNetwork, spec.listenAddress, err)
	}
	logger.Debug(ctx, "listening")

	wg.Add(1)
	go func(spec portForwardSpec) {
		defer wg.Done()
		for {
			netConn, err := l.Accept()
			if err != nil {
				// Silently ignore net.ErrClosed errors.
				if xerrors.Is(err, net.ErrClosed) {
					logger.Debug(ctx, "listener closed")
					return
				}
				_, _ = fmt.Fprintf(inv.Stderr, "Error accepting connection from '%v://%v': %v\n", spec.listenNetwork, spec.listenAddress, err)
				_, _ = fmt.Fprintln(inv.Stderr, "Killing listener")
				return
			}
			logger.Debug(ctx, "accepted connection", slog.F("remote_addr", netConn.RemoteAddr()))

			go func(netConn net.Conn) {
				defer netConn.Close()
				remoteConn, err := conn.DialContext(ctx, spec.dialNetwork, spec.dialAddress)
				if err != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "Failed to dial '%v://%v' in workspace: %s\n", spec.dialNetwork, spec.dialAddress, err)
					return
				}
				defer remoteConn.Close()
				logger.Debug(ctx, "dialed remote", slog.F("remote_addr", netConn.RemoteAddr()))

				agentssh.Bicopy(ctx, netConn, remoteConn)
				logger.Debug(ctx, "connection closing", slog.F("remote_addr", netConn.RemoteAddr()))
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
					listenAddress: port.local.String(),
					dialNetwork:   "tcp",
					dialAddress:   port.remote.String(),
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
					listenAddress: port.local.String(),
					dialNetwork:   "udp",
					dialAddress:   port.remote.String(),
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
	local, remote netip.AddrPort
}

func parseSrcDestPorts(in string) ([]parsedSrcDestPort, error) {
	var (
		err        error
		parts      = strings.Split(in, ":")
		localAddr  = netip.AddrFrom4([4]byte{127, 0, 0, 1})
		remoteAddr = netip.AddrFrom4([4]byte{127, 0, 0, 1})
	)

	switch len(parts) {
	case 1:
		// Duplicate the single part
		parts = append(parts, parts[0])
	case 2:
		// Check to see if the first part is an IP address.
		_localAddr, err := netip.ParseAddr(parts[0])
		if err != nil {
			break
		}
		// The first part is the local address, so duplicate the port.
		localAddr = _localAddr
		parts = []string{parts[1], parts[1]}

	case 3:
		_localAddr, err := netip.ParseAddr(parts[0])
		if err != nil {
			return nil, xerrors.Errorf("invalid port specification %q; invalid ip %q: %w", in, parts[0], err)
		}
		localAddr = _localAddr
		parts = parts[1:]

	default:
		return nil, xerrors.Errorf("invalid port specification %q", in)
	}

	if !strings.Contains(parts[0], "-") {
		localPort, err := parsePort(parts[0])
		if err != nil {
			return nil, xerrors.Errorf("parse local port from %q: %w", in, err)
		}
		remotePort, err := parsePort(parts[1])
		if err != nil {
			return nil, xerrors.Errorf("parse remote port from %q: %w", in, err)
		}

		return []parsedSrcDestPort{{
			local:  netip.AddrPortFrom(localAddr, localPort),
			remote: netip.AddrPortFrom(remoteAddr, remotePort),
		}}, nil
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
			local:  netip.AddrPortFrom(localAddr, local[i]),
			remote: netip.AddrPortFrom(remoteAddr, remote[i]),
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
