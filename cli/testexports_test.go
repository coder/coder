package cli

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/serpent"
)

// Export_SetupImmortalSSHStream exposes setupImmortalSSHStream for external tests.
func Export_SetupImmortalSSHStream(ctx context.Context, client *codersdk.Client, workspace codersdk.Workspace, workspaceAgent codersdk.WorkspaceAgent, logger slog.Logger, immortalFallback bool, conn workspacesdk.AgentConn) (net.Conn, *immortalStreamClient, *uuid.UUID, error) {
	return setupImmortalSSHStream(ctx, client, workspace, workspaceAgent, logger, immortalFallback, conn)
}

// Export_ListenAndPortForward exposes listenAndPortForward for external tests.
func Export_ListenAndPortForward(
	ctx context.Context,
	inv *serpent.Invocation,
	agentConn workspacesdk.AgentConn,
	wg *sync.WaitGroup,
	network string,
	listenHost netip.Addr,
	listenPort uint16,
	dialPort uint16,
	logger slog.Logger,
	immortal bool,
	immortalFallback bool,
	coderConnectHost string,
	client *codersdk.Client,
	agentID uuid.UUID,
) (net.Listener, error) {
	// Use the production helper when a valid invocation and waitgroup are provided.
	if inv != nil && wg != nil {
		spec := portForwardSpec{network: network, listenHost: listenHost, listenPort: listenPort, dialPort: dialPort}
		opts := portForwardOptions{immortal: immortal, immortalFallback: immortalFallback, coderConnectHost: coderConnectHost}
		return listenAndPortForward(ctx, inv, agentConn, wg, spec, logger, opts, client, agentID)
	}

	// Test-friendly fallback: allow nil inv/wg by using a plain net.Listener
	listenAddr := netip.AddrPortFrom(listenHost, listenPort)
	l, err := net.Listen(network, listenAddr.String())
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			netConn, aerr := l.Accept()
			if aerr != nil {
				if aerr == net.ErrClosed {
					return
				}
				logger.Error(context.Background(), "accept error", slog.Error(aerr))
				return
			}

			go func(c net.Conn) {
				defer c.Close()
				dialAddress := net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", dialPort))

				var remoteConn net.Conn
				var streamClient *immortalStreamClient
				var streamID *uuid.UUID

				if immortal && network == "tcp" {
					res, derr := DialImmortalOrFallback(ctx, agentConn, client, agentID, logger, ImmortalDialOptions{
						Enabled:          true,
						Fallback:         immortalFallback,
						TargetPort:       dialPort,
						CoderConnectHost: coderConnectHost,
					}, func(cctx context.Context) (net.Conn, error) {
						return agentConn.DialContext(cctx, network, dialAddress)
					})
					if derr != nil {
						logger.Error(context.Background(), "dial remote (immortal/fallback)", slog.Error(derr))
						return
					}
					remoteConn = res.Conn
					streamClient = res.StreamClient
					streamID = res.StreamID
				} else {
					var derr error
					remoteConn, derr = agentConn.DialContext(ctx, network, dialAddress)
					if derr != nil {
						logger.Error(context.Background(), "dial remote", slog.Error(derr))
						return
					}
				}

				defer func() {
					_ = remoteConn.Close()
					if streamClient != nil && streamID != nil {
						_ = streamClient.deleteStream(context.Background(), *streamID)
					}
				}()

				agentssh.Bicopy(ctx, c, remoteConn)
			}(netConn)
		}
	}()

	return l, nil
}
