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

// buildServerOptions constructs the NATS server Options. The embedded
// server runs as a standalone (non-clustered) instance and binds only
// a loopback client listener for wrapper-owned pub/sub connections.
func buildServerOptions(opts Options) (*natsserver.Options, error) {
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

	// Bind a loopback random client listener: the wrapper's publishPool
	// and subscribePool dial this listener via connectClient.
	sopts.DontListen = false
	sopts.Host = "127.0.0.1"
	sopts.Port = natsserver.RANDOM_PORT

	return sopts, nil
}

// startEmbeddedServer starts an in-process standalone NATS server.
func startEmbeddedServer(logger slog.Logger, opts Options) (*natsserver.Server, error) {
	sopts, err := buildServerOptions(opts)
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
	logger.Info(context.Background(), "embedded nats server started",
		slog.F("client_url", ns.ClientURL()),
	)
	return ns, nil
}

type connHandlers struct {
	disconnectErr natsgo.ConnErrHandler
	reconnect     natsgo.ConnHandler
	closed        natsgo.ConnHandler
	errH          natsgo.ErrHandler
}

// connectClient builds a NATS client that dials the embedded server's
// client listener over TCP loopback. The wrapper opens one or more
// publisher conns plus one or more subscriber conns per *Pubsub: the
// publisher pool carries all publishes (sized by Options.PublishConns),
// and the subscriber pool carries all subscriptions (sized by
// Options.SubscribeConns), with each underlying shared
// *natsgo.Subscription assigned to one subscriber conn by a stable
// hash of its subject. TCP loopback gives the server-to-client edge a
// real kernel socket buffer, which is what makes multiplexing many
// subscriptions on each subscriber conn viable.
// See docs/internal/wrapper-conn-pool-plan.md.
//
// connName is applied via natsgo.Name and identifies the connection in
// server logs (e.g., "coder-pubsub-pub", "coder-pubsub-pub-0",
// "coder-pubsub-sub", or "coder-pubsub-sub-0"). If opts.ClientName is
// set, it takes precedence.
func connectClient(ns *natsserver.Server, opts Options, handlers connHandlers, connName string) (*natsgo.Conn, error) {
	name := opts.ClientName
	if name == "" {
		name = connName
	}
	connOpts := []natsgo.Option{
		natsgo.Name(name),
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
