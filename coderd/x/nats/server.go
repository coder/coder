package nats

import (
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"golang.org/x/xerrors"
)

const readyTimeout = 10 * time.Second

// buildServerOptions constructs the embedded NATS server options. The
// server runs with a loopback random client listener and an optional
// cluster route listener.
func buildServerOptions(opts Options) (*natsserver.Options, error) {
	maxPayload := opts.MaxPayload
	if maxPayload == 0 {
		maxPayload = natsserver.MAX_PAYLOAD_SIZE
	}
	maxPending := opts.MaxPending
	if maxPending <= 0 {
		maxPending = DefaultServerMaxPendingBytes
	}

	sopts := &natsserver.Options{
		JetStream:  false,
		MaxPayload: maxPayload,
		MaxPending: maxPending,
		NoLog:      true,
		NoSigs:     true,
	}

	sopts.DontListen = false
	sopts.Host = "127.0.0.1"
	sopts.Port = natsserver.RANDOM_PORT
	if opts.ClusterAuthToken != "" {
		sopts.Authorization = opts.ClusterAuthToken
	}

	if !opts.disableCluster {
		clusterHost := opts.ClusterHost
		if clusterHost == "" {
			clusterHost = natsserver.DEFAULT_HOST
		}
		clusterPort := opts.ClusterPort
		if clusterPort == 0 {
			clusterPort = defaultClusterPort
		}
		routePoolSize := opts.RoutePoolSize
		if routePoolSize == 0 {
			routePoolSize = defaultRoutePoolSize
		}

		sopts.Cluster = natsserver.ClusterOpts{
			Name:     defaultClusterName,
			Host:     clusterHost,
			Port:     clusterPort,
			PoolSize: routePoolSize,
		}
		if opts.ClusterAuthToken != "" {
			sopts.Cluster.Username = defaultClusterTokenUsername
			sopts.Cluster.Password = opts.ClusterAuthToken
		}
	}

	return sopts, nil
}

// startEmbeddedServer starts an in-process NATS server.
func startEmbeddedServer(opts *natsserver.Options) (*natsserver.Server, error) {
	ns, err := natsserver.NewServer(opts)
	if err != nil {
		return nil, xerrors.Errorf("new embedded nats server: %w", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(readyTimeout) {
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, xerrors.Errorf("embedded nats server not ready within %s", readyTimeout)
	}
	return ns, nil
}

type connHandlers struct {
	disconnectErr natsgo.ConnErrHandler
	reconnect     natsgo.ConnHandler
	closed        natsgo.ConnHandler
	errH          natsgo.ErrHandler
}

// connectClient dials the embedded server's client listener over TCP
// loopback (or net.Pipe when opts.InProcess is true) and returns the
// resulting *natsgo.Conn. connName identifies the connection in server
// logs.
func connectClient(ns *natsserver.Server, opts Options, handlers connHandlers, connName string) (*natsgo.Conn, error) {
	connOpts := []natsgo.Option{
		natsgo.Name(connName),
		// Suppress async callbacks when we close the connection ourselves
		// during Pubsub.Close, so a graceful shutdown does not fire the
		// disconnect handler and inflate disconnections_total. Genuine
		// disconnects still invoke the handler.
		natsgo.NoCallbacksAfterClientClose(),
	}
	if opts.ClusterAuthToken != "" {
		connOpts = append(connOpts, natsgo.Token(opts.ClusterAuthToken))
	}
	if opts.ReconnectWait > 0 {
		connOpts = append(connOpts, natsgo.ReconnectWait(opts.ReconnectWait))
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
	clientURL := ns.ClientURL()
	if opts.InProcess {
		// InProcessServer overrides URL dialing with a net.Pipe; the
		// URL argument is ignored but must still be syntactically valid.
		connOpts = append(connOpts, natsgo.InProcessServer(ns))
	}
	nc, err := natsgo.Connect(clientURL, connOpts...)
	if err != nil {
		return nil, xerrors.Errorf("connect client: %w", err)
	}
	return nc, nil
}
