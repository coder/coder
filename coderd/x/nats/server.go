package nats

import (
	"fmt"
	"os"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"golang.org/x/xerrors"
)

// startEmbeddedServer starts a standalone in-process NATS server suitable
// for use with natsgo.InProcessServer. It is intentionally minimal: no
// JetStream, no listener, no clustering.
func startEmbeddedServer(opts Options) (*natsserver.Server, error) {
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
		DontListen: true,
		ServerName: serverName,
		MaxPayload: maxPayload,
		NoLog:      true,
		NoSigs:     true,
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
