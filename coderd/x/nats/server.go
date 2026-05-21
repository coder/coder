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

// buildServerOptions constructs the embedded NATS server options. The
// server runs standalone with a loopback random client listener.
func buildServerOptions(opts Options) (*natsserver.Options, error) {
	serverName := opts.ServerName
	if serverName == "" {
		serverName = fmt.Sprintf("coder-nats-%d-%d", os.Getpid(), time.Now().UnixNano())
	}
	maxPayload := opts.MaxPayload
	if maxPayload == 0 {
		maxPayload = natsserver.MAX_PAYLOAD_SIZE
	}
	// Zero => DefaultMaxPending; negative => leave zero so nats-server
	// applies its own default.
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

// connectClient dials the embedded server's client listener over TCP
// loopback (or net.Pipe when opts.InProcess is true) and returns the
// resulting *natsgo.Conn. connName identifies the connection in server
// logs; opts.ClientName overrides it when set.
func connectClient(ns *natsserver.Server, opts Options, handlers connHandlers, connName string) (*natsgo.Conn, error) {
	name := opts.ClientName
	if name == "" {
		name = connName
	}
	connOpts := []natsgo.Option{
		natsgo.Name(name),
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
		// InProcessServer overrides URL dialing with a net.Pipe; the
		// url argument is ignored but must still be syntactically valid.
		connOpts = append(connOpts, natsgo.InProcessServer(ns))
	}
	nc, err := natsgo.Connect(url, connOpts...)
	if err != nil {
		return nil, xerrors.Errorf("connect client: %w", err)
	}
	return nc, nil
}
