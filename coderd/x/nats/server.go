package nats

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

// buildServerOptions constructs the NATS server Options. The server is
// always started in cluster mode ("cluster of 1" when peers is empty)
// so that late-joining peers can be added at runtime via RefreshPeers
// without restarting the server. The peers slice is expected to already
// be normalized by normalizePeers.
//
// If opts.ClusterToken is empty, an ephemeral random token is generated
// and recorded on the returned Options so the caller can stash it back
// on the Pubsub for use by RefreshPeers.
func buildServerOptions(opts Options, peers []Peer) (*natsserver.Options, string, error) {
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

	clusterToken := opts.ClusterToken
	if clusterToken == "" {
		var buf [32]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return nil, "", xerrors.Errorf("generate ephemeral cluster token: %w", err)
		}
		clusterToken = hex.EncodeToString(buf[:])
	}

	// Bind a loopback random client listener. The embedded Coder client
	// connects to this listener over TCP loopback rather than
	// nats.InProcessServer; InProcessConn is unbuffered net.Pipe, which
	// is slow-consumer-prone under fan-out, whereas TCP loopback has
	// kernel socket buffers and is the transport upstream tunes for.
	// (Also: in nats-server v2.12.8, DontListen=true combined with a
	// non-zero Cluster.Port deadlocks the route AcceptLoop on client
	// listener readiness.)
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

	urls, err := routeURLs(peers, clusterToken)
	if err != nil {
		return nil, "", xerrors.Errorf("build route urls: %w", err)
	}

	sopts.Cluster = natsserver.ClusterOpts{
		Name:      clusterName,
		Host:      clusterHost,
		Port:      clusterPort,
		Advertise: opts.ClusterAdvertise,
		TLSConfig: opts.ClusterTLSConfig,
		PoolSize:  poolSize,
		Username:  routeAuthUsername,
		Password:  clusterToken,
	}
	sopts.Routes = urls
	sopts.CustomRouterAuthentication = &routeAuth{token: clusterToken}

	return sopts, clusterToken, nil
}

// startEmbeddedServer starts an in-process NATS server. The server is
// always started in cluster mode; with no peers this is a "cluster of
// 1" that can later be joined to peers via RefreshPeers without
// restart. The returned *natsserver.Options is the effective startup
// options used to build the server; callers may clone it (e.g., for
// ReloadOptions). The returned token is the effective cluster token,
// which may have been generated internally if opts.ClusterToken was
// empty.
func startEmbeddedServer(logger slog.Logger, opts Options, peers []Peer) (*natsserver.Server, *natsserver.Options, string, error) {
	sopts, token, err := buildServerOptions(opts, peers)
	if err != nil {
		return nil, nil, "", err
	}
	if opts.ClusterToken == "" {
		logger.Debug(context.Background(), "nats: generated ephemeral cluster token")
	}
	ns, err := natsserver.NewServer(sopts)
	if err != nil {
		return nil, nil, "", xerrors.Errorf("new embedded nats server: %w", err)
	}
	go ns.Start()
	readyTimeout := opts.ReadyTimeout
	if readyTimeout == 0 {
		readyTimeout = DefaultReadyTimeout
	}
	if !ns.ReadyForConnections(readyTimeout) {
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, nil, "", xerrors.Errorf("embedded nats server not ready within %s", readyTimeout)
	}
	logger.Info(context.Background(), "embedded nats cluster started",
		slog.F("cluster_addr", ns.ClusterAddr()),
		slog.F("peers", len(peers)),
	)
	return ns, sopts, token, nil
}

type connHandlers struct {
	disconnectErr natsgo.ConnErrHandler
	reconnect     natsgo.ConnHandler
	closed        natsgo.ConnHandler
	errH          natsgo.ErrHandler
}

// connectInProcess builds a NATS client connected to the given embedded
// server over TCP loopback (ns.ClientURL()), applying connection-level
// Options. We intentionally avoid nats.InProcessServer here: its
// transport is unbuffered net.Pipe, which causes synchronous-rendezvous
// slow-consumer failures under heavy fan-out. TCP loopback has kernel
// socket buffers and is the transport upstream benchmarks and tunes for.
func connectInProcess(ns *natsserver.Server, opts Options, handlers connHandlers) (*natsgo.Conn, error) {
	var connOpts []natsgo.Option
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
	nc, err := natsgo.Connect(ns.ClientURL(), connOpts...)
	if err != nil {
		return nil, xerrors.Errorf("connect tcp loopback: %w", err)
	}
	return nc, nil
}
