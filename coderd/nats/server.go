package nats

import (
	"context"
	"fmt"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
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
func startEmbeddedServer(logger slog.Logger, opts *natsserver.Options) (*natsserver.Server, error) {
	ns, err := natsserver.NewServer(opts)
	if err != nil {
		return nil, xerrors.Errorf("new embedded nats server: %w", err)
	}
	// Let slog filter debug messages. Protocol tracing is disabled because it
	// includes message payloads.
	ns.SetLoggerV2(serverLogger{logger: logger.Named("server")}, true, false, false)
	go ns.Start()
	if !ns.ReadyForConnections(readyTimeout) {
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, xerrors.Errorf("embedded nats server not ready within %s", readyTimeout)
	}
	return ns, nil
}

var _ natsserver.Logger = serverLogger{}

type serverLogger struct {
	logger slog.Logger
}

func (l serverLogger) Noticef(format string, args ...any) {
	slog.Helper()
	l.maybeLog(slog.LevelInfo, format, args...)
}

func (l serverLogger) Warnf(format string, args ...any) {
	slog.Helper()
	l.maybeLog(slog.LevelWarn, format, args...)
}

func (l serverLogger) Fatalf(format string, args ...any) {
	slog.Helper()
	// The embedded server must not terminate the Coder process.
	l.maybeLog(slog.LevelCritical, format, args...)
}

func (l serverLogger) Errorf(format string, args ...any) {
	slog.Helper()
	l.maybeLog(slog.LevelError, format, args...)
}

func (l serverLogger) Debugf(format string, args ...any) {
	slog.Helper()
	l.maybeLog(slog.LevelDebug, format, args...)
}

func (l serverLogger) Tracef(format string, args ...any) {
	slog.Helper()
	l.maybeLog(slog.LevelDebug, format, args...)
}

func (l serverLogger) maybeLog(level slog.Level, format string, args ...any) {
	slog.Helper()
	switch format {
	case "TLS certificate chain and hostname of solicited routes will not be verified. DO NOT USE IN PRODUCTION!":
		// This message comes from the fact that we set InsecureSkipVerify on the TLS config, which would be bad if
		// we didn't also set VerifyConnection. NATS only checks InsecureSkipVerify unfortunately, so just suppress the
		// log entirely.
		return
	case "Starting nats-server",
		"  Version:  %s",
		"  Git:      [%s]",
		"  Go build: %s",
		"  Name:     %s",
		"  ID:       %s",
		"Server is ready",
		"  Cluster:  %s",
		"Listening for client connections on %s",
		"Cluster name is %s",
		"Listening for route connections on %s":
		// These all spam on startup, and we want to not overwhelm operators running in a terminal. Reduce to debug
		// to keep the noise down but still have access to them in tests.
		level = slog.LevelDebug
	}
	l.log(level, format, args...)
}

func (l serverLogger) log(level slog.Level, format string, args ...any) {
	slog.Helper()
	switch level {
	case slog.LevelDebug:
		l.logger.Debug(context.Background(), fmt.Sprintf(format, args...))
	case slog.LevelInfo:
		l.logger.Info(context.Background(), fmt.Sprintf(format, args...))
	case slog.LevelWarn:
		l.logger.Warn(context.Background(), fmt.Sprintf(format, args...))
	case slog.LevelError:
		l.logger.Error(context.Background(), fmt.Sprintf(format, args...))
	case slog.LevelCritical:
		l.logger.Critical(context.Background(), fmt.Sprintf(format, args...))
	case slog.LevelFatal:
		l.logger.Fatal(context.Background(), fmt.Sprintf(format, args...))
	default:
		l.logger.Error(context.Background(), "unknown slog level", slog.F("level", level))
	}
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
