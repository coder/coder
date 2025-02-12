package cli

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"regexp"
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
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/serpent"
)

var (
	// noAddr is the zero-value of netip.Addr, and is not a valid address.  We use it to identify
	// when the local address is not specified in port-forward flags.
	noAddr       netip.Addr
	ipv6Loopback = netip.MustParseAddr("::1")
	ipv4Loopback = netip.MustParseAddr("127.0.0.1")
)

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

			specs, err := parsePortForwards(tcpForwards, udpForwards)
			if err != nil {
				return xerrors.Errorf("parse port-forward specs: %w", err)
			}
			if len(specs) == 0 {
				return xerrors.New("no port-forwards requested")
			}

			workspace, workspaceAgent, err := getWorkspaceAndAgent(ctx, inv, client, !disableAutostart, inv.Args[0])
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

			// Start all listeners.
			var (
				wg                = new(sync.WaitGroup)
				listeners         = make([]net.Listener, 0, len(specs)*2)
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

			for _, spec := range specs {
				if spec.listenHost == noAddr {
					// first, opportunistically try to listen on IPv6
					spec6 := spec
					spec6.listenHost = ipv6Loopback
					l6, err6 := listenAndPortForward(ctx, inv, conn, wg, spec6, logger)
					if err6 != nil {
						logger.Info(ctx, "failed to opportunistically listen on IPv6", slog.F("spec", spec), slog.Error(err6))
					} else {
						listeners = append(listeners, l6)
					}
					spec.listenHost = ipv4Loopback
				}
				l, err := listenAndPortForward(ctx, inv, conn, wg, spec, logger)
				if err != nil {
					logger.Error(ctx, "failed to listen", slog.F("spec", spec), slog.Error(err))
					return err
				}
				listeners = append(listeners, l)
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
	conn *workspacesdk.AgentConn,
	wg *sync.WaitGroup,
	spec portForwardSpec,
	logger slog.Logger,
) (net.Listener, error) {
	logger = logger.With(
		slog.F("network", spec.network),
		slog.F("listen_host", spec.listenHost),
		slog.F("listen_port", spec.listenPort),
	)
	listenAddress := netip.AddrPortFrom(spec.listenHost, spec.listenPort)
	dialAddress := fmt.Sprintf("127.0.0.1:%d", spec.dialPort)
	_, _ = fmt.Fprintf(inv.Stderr, "Forwarding '%s://%s' locally to '%s://%s' in the workspace\n",
		spec.network, listenAddress, spec.network, dialAddress)

	l, err := inv.Net.Listen(spec.network, listenAddress.String())
	if err != nil {
		return nil, xerrors.Errorf("listen '%s://%s': %w", spec.network, listenAddress.String(), err)
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
				_, _ = fmt.Fprintf(inv.Stderr,
					"Error accepting connection from '%s://%s': %v\n",
					spec.network, listenAddress.String(), err)
				_, _ = fmt.Fprintln(inv.Stderr, "Killing listener")
				return
			}
			logger.Debug(ctx, "accepted connection",
				slog.F("remote_addr", netConn.RemoteAddr()))

			go func(netConn net.Conn) {
				defer netConn.Close()
				remoteConn, err := conn.DialContext(ctx, spec.network, dialAddress)
				if err != nil {
					_, _ = fmt.Fprintf(inv.Stderr,
						"Failed to dial '%s://%s' in workspace: %s\n",
						spec.network, dialAddress, err)
					return
				}
				defer remoteConn.Close()
				logger.Debug(ctx,
					"dialed remote", slog.F("remote_addr", netConn.RemoteAddr()))

				agentssh.Bicopy(ctx, netConn, remoteConn)
				logger.Debug(ctx,
					"connection closing", slog.F("remote_addr", netConn.RemoteAddr()))
			}(netConn)
		}
	}(spec)

	return l, nil
}

type portForwardSpec struct {
	network              string // tcp, udp
	listenHost           netip.Addr
	listenPort, dialPort uint16
}

func parsePortForwards(tcpSpecs, udpSpecs []string) ([]portForwardSpec, error) {
	specs := []portForwardSpec{}

	for _, specEntry := range tcpSpecs {
		for _, spec := range strings.Split(specEntry, ",") {
			pfSpecs, err := parseSrcDestPorts(strings.TrimSpace(spec))
			if err != nil {
				return nil, xerrors.Errorf("failed to parse TCP port-forward specification %q: %w", spec, err)
			}

			for _, pfSpec := range pfSpecs {
				pfSpec.network = "tcp"
				specs = append(specs, pfSpec)
			}
		}
	}

	for _, specEntry := range udpSpecs {
		for _, spec := range strings.Split(specEntry, ",") {
			pfSpecs, err := parseSrcDestPorts(strings.TrimSpace(spec))
			if err != nil {
				return nil, xerrors.Errorf("failed to parse UDP port-forward specification %q: %w", spec, err)
			}

			for _, pfSpec := range pfSpecs {
				pfSpec.network = "udp"
				specs = append(specs, pfSpec)
			}
		}
	}

	// Check for duplicate entries.
	locals := map[string]struct{}{}
	for _, spec := range specs {
		localStr := fmt.Sprintf("%s:%s:%d", spec.network, spec.listenHost, spec.listenPort)
		if _, ok := locals[localStr]; ok {
			return nil, xerrors.Errorf("local %s host:%s port:%d is specified twice", spec.network, spec.listenHost, spec.listenPort)
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

// specRegexp matches port specs. It handles all the following formats:
//
// 8000
// 8888:9999
// 1-5:6-10
// 8000-8005
// 127.0.0.1:4000:4000
// [::1]:8080:8081
// 127.0.0.1:4000-4005
// [::1]:4000-4001:5000-5001
//
// Important capturing groups:
//
// 2: local IP address (including [] for IPv6)
// 3: local port, or start of local port range
// 5: end of local port range
// 7: remote port, or start of remote port range
// 9: end or remote port range
var specRegexp = regexp.MustCompile(`^((\[[0-9a-fA-F:]+]|\d+\.\d+\.\d+\.\d+):)?(\d+)(-(\d+))?(:(\d+)(-(\d+))?)?$`)

func parseSrcDestPorts(in string) ([]portForwardSpec, error) {
	groups := specRegexp.FindStringSubmatch(in)
	if len(groups) == 0 {
		return nil, xerrors.Errorf("invalid port specification %q", in)
	}

	var localAddr netip.Addr
	if groups[2] != "" {
		parsedAddr, err := netip.ParseAddr(strings.Trim(groups[2], "[]"))
		if err != nil {
			return nil, xerrors.Errorf("invalid IP address %q", groups[2])
		}
		localAddr = parsedAddr
	}

	local, err := parsePortRange(groups[3], groups[5])
	if err != nil {
		return nil, xerrors.Errorf("parse local port range from %q: %w", in, err)
	}
	remote := local
	if groups[7] != "" {
		remote, err = parsePortRange(groups[7], groups[9])
		if err != nil {
			return nil, xerrors.Errorf("parse remote port range from %q: %w", in, err)
		}
	}
	if len(local) != len(remote) {
		return nil, xerrors.Errorf("port ranges must be the same length, got %d ports forwarded to %d ports", len(local), len(remote))
	}
	var out []portForwardSpec
	for i := range local {
		out = append(out, portForwardSpec{
			listenHost: localAddr,
			listenPort: local[i],
			dialPort:   remote[i],
		})
	}
	return out, nil
}

func parsePortRange(s, e string) ([]uint16, error) {
	start, err := parsePort(s)
	if err != nil {
		return nil, xerrors.Errorf("parse range start port from %q: %w", s, err)
	}
	end := start
	if len(e) != 0 {
		end, err = parsePort(e)
		if err != nil {
			return nil, xerrors.Errorf("parse range end port from %q: %w", e, err)
		}
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
