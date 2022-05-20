package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/pion/udp"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	coderagent "github.com/coder/coder/agent"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func portForward() *cobra.Command {
	var (
		tcpForwards  []string // <port>:<port>
		udpForwards  []string // <port>:<port>
		unixForwards []string // <path>:<path> OR <port>:<path>
	)
	cmd := &cobra.Command{
		Use:     "port-forward <workspace>",
		Aliases: []string{"tunnel"},
		Args:    cobra.ExactArgs(1),
		Example: `
  - Port forward a single TCP port from 1234 in the workspace to port 5678 on
    your local machine

    ` + cliui.Styles.Code.Render("$ coder port-forward <workspace> --tcp 5678:1234") + `

  - Port forward a single UDP port from port 9000 to port 9000 on your local
    machine

    ` + cliui.Styles.Code.Render("$ coder port-forward <workspace> --udp 9000") + `

  - Forward a Unix socket in the workspace to a local Unix socket

    ` + cliui.Styles.Code.Render("$ coder port-forward <workspace> --unix ./local.sock:~/remote.sock") + `

  - Forward a Unix socket in the workspace to a local TCP port

    ` + cliui.Styles.Code.Render("$ coder port-forward <workspace> --unix 8080:~/remote.sock") + `

  - Port forward multiple TCP ports and a UDP port

    ` + cliui.Styles.Code.Render("$ coder port-forward <workspace> --tcp 8080:8080 --tcp 9000:3000 --udp 5353:53"),
		RunE: func(cmd *cobra.Command, args []string) error {
			specs, err := parsePortForwards(tcpForwards, udpForwards, unixForwards)
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
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}

			workspace, agent, err := getWorkspaceAndAgent(cmd, client, organization.ID, codersdk.Me, args[0], false)
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
					return client.WorkspaceAgent(ctx, agent.ID)
				},
			})
			if err != nil {
				return xerrors.Errorf("await agent: %w", err)
			}

			conn, err := client.DialWorkspaceAgent(cmd.Context(), agent.ID, nil)
			if err != nil {
				return xerrors.Errorf("dial workspace agent: %w", err)
			}
			defer conn.Close()

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
				l, err := listenAndPortForward(ctx, cmd, conn, wg, spec)
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
	cmd.Flags().StringArrayVar(&unixForwards, "unix", []string{}, "Forward a Unix socket in the workspace to a local Unix socket or TCP port")

	return cmd
}

func listenAndPortForward(ctx context.Context, cmd *cobra.Command, conn *coderagent.Conn, wg *sync.WaitGroup, spec portForwardSpec) (net.Listener, error) {
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
	case "unix":
		l, err = net.Listen(spec.listenNetwork, spec.listenAddress)
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

				coderagent.Bicopy(ctx, netConn, remoteConn)
			}(netConn)
		}
	}(spec)

	return l, nil
}

type portForwardSpec struct {
	listenNetwork string // tcp, udp, unix
	listenAddress string // <ip>:<port> or path

	dialNetwork string // tcp, udp, unix
	dialAddress string // <ip>:<port> or path
}

func parsePortForwards(tcpSpecs, udpSpecs, unixSpecs []string) ([]portForwardSpec, error) {
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

	for _, specStr := range unixSpecs {
		localPath, localTCP, remotePath, err := parseUnixUnix(specStr)
		if err != nil {
			return nil, xerrors.Errorf("failed to parse Unix port-forward specification %q: %w", specStr, err)
		}

		spec := portForwardSpec{
			dialNetwork: "unix",
			dialAddress: remotePath,
		}
		if localPath == "" {
			spec.listenNetwork = "tcp"
			spec.listenAddress = fmt.Sprintf("127.0.0.1:%v", localTCP)
		} else {
			if runtime.GOOS == "windows" {
				return nil, xerrors.Errorf("Unix port-forwarding is not supported on Windows")
			}
			spec.listenNetwork = "unix"
			spec.listenAddress = localPath
		}
		specs = append(specs, spec)
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

func parseUnixPath(in string) (string, error) {
	path, err := coderagent.ExpandRelativeHomePath(strings.TrimSpace(in))
	if err != nil {
		return "", xerrors.Errorf("tidy path %q: %w", in, err)
	}

	return path, nil
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

func parsePortOrUnixPath(in string) (string, uint16, error) {
	port, err := parsePort(in)
	if err == nil {
		return "", port, nil
	}

	path, err := parseUnixPath(in)
	if err != nil {
		return "", 0, xerrors.Errorf("could not parse port or unix path %q: %w", in, err)
	}

	return path, 0, nil
}

func parseUnixUnix(in string) (string, uint16, string, error) {
	parts := strings.Split(in, ":")
	if len(parts) > 2 {
		return "", 0, "", xerrors.Errorf("invalid port-forward specification %q", in)
	}
	if len(parts) == 1 {
		// Duplicate the single part
		parts = append(parts, parts[0])
	}

	localPath, localPort, err := parsePortOrUnixPath(parts[0])
	if err != nil {
		return "", 0, "", xerrors.Errorf("parse local part of spec %q: %w", in, err)
	}

	// We don't really touch the remote path at all since it gets cleaned
	// up/expanded on the remote.
	return localPath, localPort, parts[1], nil
}
