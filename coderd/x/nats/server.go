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

	// MaxPending: zero means use DefaultMaxPending (1 GiB). Negative
	// means use the nats-server default by leaving the field at zero
	// (the server fills in 64 MiB during processOptions).
	maxPending := opts.MaxPending
	switch {
	case maxPending == 0:
		maxPending = DefaultMaxPending
	case maxPending < 0:
		maxPending = 0
	}

	sopts := &natsserver.Options{
		JetStream:  false,
		ServerName: serverName,
		MaxPayload: maxPayload,
		MaxPending: maxPending,
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

	// Bind a loopback random client listener: the wrapper's pubConn
	// and subConn dial this listener via connectClient. Additionally,
	// nats-server v2.12.8 deadlocks the route AcceptLoop on client
	// listener readiness when DontListen=true is combined with a
	// non-zero Cluster.Port, so the listener must be real.
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

// connectClient builds a NATS client that dials the embedded server's
// client listener over TCP loopback. The wrapper opens exactly two of
// these per *Pubsub: pubConn for all publishes, subConn for all
// subscriptions. TCP loopback gives the server-to-client edge a real
// kernel socket buffer, which is what makes multiplexing many
// subscriptions on a single subConn viable. See
// docs/internal/wrapper-conn-pool-plan.md.
//
// connName is applied via natsgo.Name and identifies the connection in
// server logs (e.g., "coder-pubsub-pub" or "coder-pubsub-sub"). If
// opts.ClientName is set, it takes precedence.
func connectClient(ns *natsserver.Server, opts Options, handlers connHandlers, connName string) (*natsgo.Conn, error) {
	name := opts.ClientName
	if name == "" {
		name = connName
	}
	connOpts := []natsgo.Option{
		natsgo.Name(name),
		// The server lives in this same process; treat any disconnect as
		// transient and reconnect indefinitely.
		natsgo.MaxReconnects(-1),
		// All publish subjects on connections owned by this wrapper are
		// produced by LegacyEventSubject / BuildSubject, which have
		// already validated the subject. Skip the redundant per-publish
		// validation inside nats.go to keep the hot path lean.
		natsgo.SkipSubjectValidation(),
	}
	if opts.DrainTimeout > 0 {
		connOpts = append(connOpts, natsgo.DrainTimeout(opts.DrainTimeout))
	}
	if opts.ReconnectWait > 0 {
		connOpts = append(connOpts, natsgo.ReconnectWait(opts.ReconnectWait))
	}
	// Allow callers to override MaxReconnects if they supplied an
	// explicit non-zero value.
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
	url := ns.ClientURL()
	if opts.InProcess {
		// InProcessServer overrides the URL dial with a net.Pipe
		// directly into the server. The url argument to Connect is
		// ignored in that case but must still be syntactically valid.
		connOpts = append(connOpts, natsgo.InProcessServer(ns))
	}
	nc, err := natsgo.Connect(url, connOpts...)
	if err != nil {
		return nil, xerrors.Errorf("connect client: %w", err)
	}
	return nc, nil
}
