package reconnectingpty

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type Server struct {
	logger           slog.Logger
	connectionsTotal prometheus.Counter
	errorsTotal      *prometheus.CounterVec
	commandCreator   *agentssh.Server
	connCount        atomic.Int64
	reconnectingPTYs sync.Map
	timeout          time.Duration
}

// NewServer returns a new ReconnectingPTY server
func NewServer(logger slog.Logger, commandCreator *agentssh.Server,
	connectionsTotal prometheus.Counter, errorsTotal *prometheus.CounterVec,
	timeout time.Duration,
) *Server {
	return &Server{
		logger:           logger,
		commandCreator:   commandCreator,
		connectionsTotal: connectionsTotal,
		errorsTotal:      errorsTotal,
		timeout:          timeout,
	}
}

func (s *Server) Serve(ctx, hardCtx context.Context, l net.Listener) (retErr error) {
	var wg sync.WaitGroup
	for {
		if ctx.Err() != nil {
			break
		}
		conn, err := l.Accept()
		if err != nil {
			s.logger.Debug(ctx, "accept pty failed", slog.Error(err))
			retErr = err
			break
		}
		clog := s.logger.With(
			slog.F("remote", conn.RemoteAddr().String()),
			slog.F("local", conn.LocalAddr().String()))
		clog.Info(ctx, "accepted conn")
		wg.Add(1)
		closed := make(chan struct{})
		go func() {
			select {
			case <-closed:
			case <-hardCtx.Done():
				_ = conn.Close()
			}
			wg.Done()
		}()
		wg.Add(1)
		go func() {
			defer close(closed)
			defer wg.Done()
			_ = s.handleConn(ctx, clog, conn)
		}()
	}
	wg.Wait()
	return retErr
}

func (s *Server) ConnCount() int64 {
	return s.connCount.Load()
}

func (s *Server) handleConn(ctx context.Context, logger slog.Logger, conn net.Conn) (retErr error) {
	defer conn.Close()
	s.connectionsTotal.Add(1)
	s.connCount.Add(1)
	defer s.connCount.Add(-1)

	// This cannot use a JSON decoder, since that can
	// buffer additional data that is required for the PTY.
	rawLen := make([]byte, 2)
	_, err := conn.Read(rawLen)
	if err != nil {
		// logging at info since a single incident isn't too worrying (the client could just have
		// hung up), but if we get a lot of these we'd want to investigate.
		logger.Info(ctx, "failed to read AgentReconnectingPTYInit length", slog.Error(err))
		return nil
	}
	length := binary.LittleEndian.Uint16(rawLen)
	data := make([]byte, length)
	_, err = conn.Read(data)
	if err != nil {
		// logging at info since a single incident isn't too worrying (the client could just have
		// hung up), but if we get a lot of these we'd want to investigate.
		logger.Info(ctx, "failed to read AgentReconnectingPTYInit", slog.Error(err))
		return nil
	}
	var msg workspacesdk.AgentReconnectingPTYInit
	err = json.Unmarshal(data, &msg)
	if err != nil {
		logger.Warn(ctx, "failed to unmarshal init", slog.F("raw", data))
		return nil
	}

	connectionID := uuid.NewString()
	connLogger := logger.With(slog.F("message_id", msg.ID), slog.F("connection_id", connectionID), slog.F("container", msg.Container), slog.F("container_user", msg.ContainerUser))
	connLogger.Debug(ctx, "starting handler")

	defer func() {
		if err := retErr; err != nil {
			// If the context is done, we don't want to log this as an error since it's expected.
			if ctx.Err() != nil {
				connLogger.Info(ctx, "reconnecting pty failed with attach error (agent closed)", slog.Error(err))
			} else {
				connLogger.Error(ctx, "reconnecting pty failed with attach error", slog.Error(err))
			}
		}
		connLogger.Info(ctx, "reconnecting pty connection closed")
	}()

	var rpty ReconnectingPTY
	sendConnected := make(chan ReconnectingPTY, 1)
	// On store, reserve this ID to prevent multiple concurrent new connections.
	waitReady, ok := s.reconnectingPTYs.LoadOrStore(msg.ID, sendConnected)
	if ok {
		close(sendConnected) // Unused.
		connLogger.Debug(ctx, "connecting to existing reconnecting pty")
		c, ok := waitReady.(chan ReconnectingPTY)
		if !ok {
			return xerrors.Errorf("found invalid type in reconnecting pty map: %T", waitReady)
		}
		rpty, ok = <-c
		if !ok || rpty == nil {
			return xerrors.Errorf("reconnecting pty closed before connection")
		}
		c <- rpty // Put it back for the next reconnect.
	} else {
		connLogger.Debug(ctx, "creating new reconnecting pty")

		connected := false
		defer func() {
			if !connected && retErr != nil {
				s.reconnectingPTYs.Delete(msg.ID)
				close(sendConnected)
			}
		}()

		var ei agentssh.EnvInfoer
		if msg.Container != "" {
			dei, err := agentcontainers.EnvInfo(ctx, s.commandCreator.Execer, msg.Container, msg.ContainerUser)
			if err != nil {
				return xerrors.Errorf("get container env info: %w", err)
			}
			ei = dei
			s.logger.Info(ctx, "got container env info", slog.F("container", msg.Container))
		}
		// Empty command will default to the users shell!
		cmd, err := s.commandCreator.CreateCommand(ctx, msg.Command, nil, ei)
		if err != nil {
			s.errorsTotal.WithLabelValues("create_command").Add(1)
			return xerrors.Errorf("create command: %w", err)
		}

		rpty = New(ctx,
			logger.With(slog.F("message_id", msg.ID)),
			s.commandCreator.Execer,
			cmd,
			&Options{
				Timeout: s.timeout,
				Metrics: s.errorsTotal,
			},
		)

		done := make(chan struct{})
		go func() {
			select {
			case <-done:
			case <-ctx.Done():
				rpty.Close(ctx.Err())
			}
		}()

		go func() {
			rpty.Wait()
			s.reconnectingPTYs.Delete(msg.ID)
		}()

		connected = true
		sendConnected <- rpty
	}
	return rpty.Attach(ctx, connectionID, conn, msg.Height, msg.Width, connLogger)
}
