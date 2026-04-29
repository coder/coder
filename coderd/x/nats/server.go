package nats

import (
	"context"
	"fmt"
	"os"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// routeAuth is a minimal CustomRouterAuthentication shim. NATS' built-in
// route authentication compares a CONNECT-supplied user/pass against
// Cluster.Username/Password, but with our scheme the authoritative
// secret is the shared cluster token. Accept any CONNECT whose password
// equals the configured token; ignore the username (we always set it to
// routeAuthUsername on outbound URLs).
type routeAuth struct {
	token string
}

func (a *routeAuth) Check(c natsserver.ClientAuthentication) bool {
	if a.token == "" {
		return false
	}
	return c.GetOpts().Password == a.token
}

// buildServerOptions constructs the NATS server Options for either
// standalone (no peers) or cluster mode (>=1 peer). The peers slice is
// expected to already be normalized by normalizePeers.
func buildServerOptions(opts Options, peers []Peer) (*natsserver.Options, error) {
	serverName := opts.ServerName
	if serverName == "" {
		serverName = fmt.Sprintf("coder-nats-%d-%d", os.Getpid(), time.Now().UnixNano())
	}
	maxPayload := opts.MaxPayload
	if maxPayload == 0 {
		maxPayload = natsserver.MAX_PAYLOAD_SIZE
	}

	sopts := &natsserver.Options{
		JetStream:  false,
		ServerName: serverName,
		MaxPayload: maxPayload,
		NoLog:      true,
		NoSigs:     true,
	}

	if len(peers) == 0 {
		// Standalone: no listener, no cluster.
		sopts.DontListen = true
		return sopts, nil
	}

	if opts.ClusterToken == "" {
		return nil, xerrors.New("ClusterToken is required when peers are configured")
	}

	// NOTE: in nats-server v2.12.8, DontListen=true combined with a
	// non-zero Cluster.Port deadlocks the route AcceptLoop on client
	// listener readiness. Bind a loopback random client listener; the
	// embedded Coder client still connects via InProcessServer.
	sopts.DontListen = false
	sopts.Host = "127.0.0.1"
	sopts.Port = natsserver.RANDOM_PORT

	clusterName := opts.ClusterName
	if clusterName == "" {
		clusterName = DefaultClusterName
	}
	clusterHost := opts.ClusterHost
	if clusterHost == "" {
		clusterHost = "127.0.0.1"
	}
	// Cluster.Port==0 means "disable routes" in nats-server. Translate
	// the user-friendly zero to RANDOM_PORT to ensure the cluster
	// listener actually binds.
	clusterPort := opts.ClusterPort
	if clusterPort == 0 {
		clusterPort = natsserver.RANDOM_PORT
	}
	poolSize := opts.RoutePoolSize
	if poolSize == 0 {
		poolSize = DefaultRoutePoolSize
	}

	urls, err := routeURLs(peers, opts.ClusterToken)
	if err != nil {
		return nil, xerrors.Errorf("build route urls: %w", err)
	}

	sopts.Cluster = natsserver.ClusterOpts{
		Name:      clusterName,
		Host:      clusterHost,
		Port:      clusterPort,
		Advertise: opts.ClusterAdvertise,
		TLSConfig: opts.ClusterTLSConfig,
		PoolSize:  poolSize,
		Username:  routeAuthUsername,
		Password:  opts.ClusterToken,
	}
	sopts.Routes = urls
	sopts.CustomRouterAuthentication = &routeAuth{token: opts.ClusterToken}

	return sopts, nil
}

// startEmbeddedServer starts an in-process NATS server. With no peers it
// runs standalone (no listener, no cluster). With peers it joins a
// cluster using shared-token route authentication.
func startEmbeddedServer(logger slog.Logger, opts Options, peers []Peer) (*natsserver.Server, error) {
	sopts, err := buildServerOptions(opts, peers)
	if err != nil {
		return nil, err
	}
	ns, err := natsserver.NewServer(sopts)
	if err != nil {
		return nil, xerrors.Errorf("new embedded nats server: %w", err)
	}
	go ns.Start()
	readyTimeout := opts.ReadyTimeout
	if readyTimeout == 0 {
		readyTimeout = DefaultReadyTimeout
	}
	if !ns.ReadyForConnections(readyTimeout) {
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, xerrors.Errorf("embedded nats server not ready within %s", readyTimeout)
	}
	if len(peers) > 0 {
		logger.Info(context.Background(), "embedded nats cluster started",
			slog.F("cluster_addr", ns.ClusterAddr()),
			slog.F("peers", len(peers)),
		)
	}
	return ns, nil
}

type connHandlers struct {
	disconnectErr natsgo.ConnErrHandler
	reconnect     natsgo.ConnHandler
	closed        natsgo.ConnHandler
	errH          natsgo.ErrHandler
}

// connectInProcess builds a NATS client connected in-process to the given
// embedded server, applying connection-level Options.
func connectInProcess(ns *natsserver.Server, opts Options, handlers connHandlers) (*natsgo.Conn, error) {
	connOpts := []natsgo.Option{natsgo.InProcessServer(ns)}
	if opts.ClientName != "" {
		connOpts = append(connOpts, natsgo.Name(opts.ClientName))
	}
	if opts.NoEcho {
		connOpts = append(connOpts, natsgo.NoEcho())
	}
	if opts.DrainTimeout > 0 {
		connOpts = append(connOpts, natsgo.DrainTimeout(opts.DrainTimeout))
	}
	if opts.ReconnectWait > 0 {
		connOpts = append(connOpts, natsgo.ReconnectWait(opts.ReconnectWait))
	}
	if opts.MaxReconnects != 0 {
		connOpts = append(connOpts, natsgo.MaxReconnects(opts.MaxReconnects))
	}
	if handlers.disconnectErr != nil {
		connOpts = append(connOpts, natsgo.DisconnectErrHandler(handlers.disconnectErr))
	}
	if handlers.reconnect != nil {
		connOpts = append(connOpts, natsgo.ReconnectHandler(handlers.reconnect))
	}
	if handlers.closed != nil {
		connOpts = append(connOpts, natsgo.ClosedHandler(handlers.closed))
	}
	if handlers.errH != nil {
		connOpts = append(connOpts, natsgo.ErrorHandler(handlers.errH))
	}
	nc, err := natsgo.Connect("", connOpts...)
	if err != nil {
		return nil, xerrors.Errorf("connect in-process: %w", err)
	}
	return nc, nil
}
