package immortalstreams

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/immortalstreams/backedpipe"
	"github.com/coder/coder/v2/codersdk"
)

// Stream represents an immortal stream connection
type Stream struct {
	id        uuid.UUID
	name      string
	port      int
	createdAt time.Time
	logger    slog.Logger

	mu                  sync.RWMutex
	localConn           io.ReadWriteCloser
	pipe                *backedpipe.BackedPipe
	lastConnectionAt    time.Time
	lastDisconnectionAt time.Time
	connected           bool
	closed              bool

	// Indicates a reconnect handshake is in progress (from pending request
	// until the pipe reports connected). Prevents a second ForceReconnect
	// from racing and closing the just-provided connection.
	handshakePending bool

	// goroutines manages the copy goroutines
	goroutines sync.WaitGroup

	// Reconnection coordination
	pendingReconnect *reconnectRequest
	// Condition variable to wait for pendingReconnect changes
	reconnectCond *sync.Cond

	// Reconnect worker signaling (coalesced pokes)
	reconnectReq chan struct{}

	// Disconnection detection
	disconnectChan chan struct{}

	// Shutdown signal
	shutdownChan chan struct{}
}

// reconnectRequest represents a pending reconnection request
type reconnectRequest struct {
	writerSeqNum uint64
	response     chan reconnectResponse
}

// reconnectResponse represents a reconnection response
type reconnectResponse struct {
	conn    io.ReadWriteCloser
	readSeq uint64
	err     error
}

// NewStream creates a new immortal stream
func NewStream(id uuid.UUID, name string, port int, logger slog.Logger) *Stream {
	stream := &Stream{
		id:             id,
		name:           name,
		port:           port,
		createdAt:      time.Now(),
		logger:         logger,
		disconnectChan: make(chan struct{}, 1),
		shutdownChan:   make(chan struct{}),
		reconnectReq:   make(chan struct{}, 1),
	}
	stream.reconnectCond = sync.NewCond(&stream.mu)

	// Create a reconnect function that waits for a client connection
	reconnectFn := func(ctx context.Context, writerSeqNum uint64) (io.ReadWriteCloser, uint64, error) {
		// Wait for HandleReconnect to be called with a new connection
		responseChan := make(chan reconnectResponse, 1)

		stream.mu.Lock()
		stream.pendingReconnect = &reconnectRequest{
			writerSeqNum: writerSeqNum,
			response:     responseChan,
		}
		stream.handshakePending = true
		// Mark disconnected if we previously had a client connection
		if stream.connected {
			stream.connected = false
			stream.lastDisconnectionAt = time.Now()
		}
		stream.logger.Info(context.Background(), "pending reconnect set",
			slog.F("writer_seq", writerSeqNum))
		// Signal waiters a reconnect request is pending
		stream.reconnectCond.Broadcast()
		stream.mu.Unlock()

		// Fast path: if the stream is already shutting down, abort immediately
		select {
		case <-stream.shutdownChan:
			stream.mu.Lock()
			// Clear the pending request since we're aborting
			if stream.pendingReconnect != nil {
				stream.pendingReconnect = nil
			}
			stream.mu.Unlock()
			return nil, 0, xerrors.New("stream is shutting down")
		default:
		}

		// Wait for response from HandleReconnect or context cancellation
		stream.logger.Info(context.Background(), "reconnect function waiting for response")
		select {
		case resp := <-responseChan:
			stream.logger.Info(context.Background(), "reconnect function got response",
				slog.F("has_conn", resp.conn != nil),
				slog.F("read_seq", resp.readSeq),
				slog.Error(resp.err))
			return resp.conn, resp.readSeq, resp.err
		case <-ctx.Done():
			// Context was canceled, clear pending request and return error
			stream.mu.Lock()
			stream.pendingReconnect = nil
			stream.handshakePending = false
			stream.mu.Unlock()
			return nil, 0, ctx.Err()
		case <-stream.shutdownChan:
			// Stream is being shut down, clear pending request and return error
			stream.mu.Lock()
			stream.pendingReconnect = nil
			stream.handshakePending = false
			stream.mu.Unlock()
			return nil, 0, xerrors.New("stream is shutting down")
		}
	}

	// Create BackedPipe with background context
	stream.pipe = backedpipe.NewBackedPipe(context.Background(), reconnectFn)

	// Start reconnect worker: dedupe pokes and call ForceReconnect when safe.
	go func() {
		for {
			select {
			case <-stream.shutdownChan:
				return
			case <-stream.reconnectReq:
				// Drain extra pokes to coalesce
				for {
					select {
					case <-stream.reconnectReq:
					default:
						goto drained
					}
				}
			drained:
				stream.mu.Lock()
				closed := stream.closed
				handshaking := stream.handshakePending
				canReconnect := stream.pipe != nil && !stream.pipe.Connected()
				stream.mu.Unlock()
				if closed || handshaking || !canReconnect {
					// Nothing to do now; wait for a future poke.
					continue
				}
				// BackedPipe handles singleflight internally.
				stream.logger.Debug(context.Background(), "worker calling ForceReconnect")
				err := stream.pipe.ForceReconnect()
				stream.logger.Debug(context.Background(), "worker ForceReconnect returned", slog.Error(err))
				// Wake any waiters to re-check state after attempt completes.
				stream.mu.Lock()
				if stream.reconnectCond != nil {
					stream.reconnectCond.Broadcast()
				}
				stream.mu.Unlock()
			}
		}
	}()

	return stream
}

// Start starts the stream with an initial connection
func (s *Stream) Start(localConn io.ReadWriteCloser) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return xerrors.New("stream is closed")
	}

	s.localConn = localConn
	s.lastConnectionAt = time.Now()
	s.connected = false // Not connected to client yet

	// Start copying data between the local connection and the backed pipe
	s.startCopyingLocked()

	return nil
}

// HandleReconnect handles a client reconnection
func (s *Stream) HandleReconnect(clientConn io.ReadWriteCloser, readSeqNum uint64) error {
	s.mu.Lock()

	if s.closed {
		s.mu.Unlock()
		return xerrors.New("stream is closed")
	}

	s.logger.Info(context.Background(), "handling reconnection",
		slog.F("read_seq_num", readSeqNum),
		slog.F("has_pending", s.pendingReconnect != nil))

	// Helper: request a reconnect attempt by poking the worker
	requestReconnect := func() {
		select {
		case s.reconnectReq <- struct{}{}:
		default:
			// already requested; coalesced
		}
	}

	// Main coordination loop. Use a proper cond.Wait loop to avoid lost wakeups.
	for {
		// If a reconnect request is pending, respond with this connection.
		if s.pendingReconnect != nil {
			s.logger.Debug(context.Background(), "responding to pending reconnect",
				slog.F("read_seq", readSeqNum))
			respCh := s.pendingReconnect.response
			s.pendingReconnect = nil
			// Release the lock before sending to avoid blocking other goroutines.
			s.mu.Unlock()
			respCh <- reconnectResponse{conn: clientConn, readSeq: readSeqNum, err: nil}

			// Wait until the pipe reports a connected state so the handshake fully completes.
			// Use a bounded timeout to avoid hanging forever in pathological cases.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := s.pipe.WaitForConnection(ctx)
			cancel()
			if err != nil {
				s.mu.Lock()
				s.connected = false
				if s.reconnectCond != nil {
					s.reconnectCond.Broadcast()
				}
				s.mu.Unlock()
				s.logger.Warn(context.Background(), "failed to connect backed pipe", slog.Error(err))
				return xerrors.Errorf("failed to establish connection: %w", err)
			}

			s.mu.Lock()
			s.lastConnectionAt = time.Now()
			s.connected = true
			s.handshakePending = false
			if s.reconnectCond != nil {
				s.reconnectCond.Broadcast()
			}
			s.mu.Unlock()

			s.logger.Debug(context.Background(), "client reconnection successful")
			return nil
		}

		// If closed, abort.
		if s.closed {
			s.mu.Unlock()
			return xerrors.New("stream is closed")
		}

		// If already connected, another goroutine handled it; report back.
		if s.connected {
			s.mu.Unlock()
			s.logger.Debug(context.Background(), "another goroutine completed reconnection")
			return xerrors.New("stream is already connected")
		}

		// Ensure a reconnect attempt is requested while we wait.
		requestReconnect()

		// Wait until state changes: pendingReconnect set, connection established, or closed.
		s.logger.Debug(context.Background(), "waiting for pending request or connection change",
			slog.F("pending", s.pendingReconnect != nil),
			slog.F("connected", s.connected),
			slog.F("closed", s.closed))
		s.reconnectCond.Wait()
		// Loop will re-check conditions under lock to avoid lost wakeups.
	}
}

// Close closes the stream
func (s *Stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	s.connected = false

	// Signal shutdown to any pending reconnect attempts and listeners
	// Closing the channel wakes all waiters exactly once
	select {
	case <-s.shutdownChan:
		// already closed
	default:
		close(s.shutdownChan)
	}

	// Wake any goroutines waiting for a pending reconnect request so they
	// observe the closed state and exit promptly.
	if s.reconnectCond != nil {
		s.reconnectCond.Broadcast()
	}

	// Clear any pending reconnect request
	if s.pendingReconnect != nil {
		s.pendingReconnect.response <- reconnectResponse{
			conn:    nil,
			readSeq: 0,
			err:     xerrors.New("stream is shutting down"),
		}
		s.pendingReconnect = nil
		s.handshakePending = false
	}

	// Close the backed pipe
	if s.pipe != nil {
		if err := s.pipe.Close(); err != nil {
			s.logger.Warn(context.Background(), "failed to close backed pipe", slog.Error(err))
		}
	}

	// Close connections
	if s.localConn != nil {
		if err := s.localConn.Close(); err != nil {
			s.logger.Warn(context.Background(), "failed to close local connection", slog.Error(err))
		}
	}

	// Wait for goroutines to finish
	s.mu.Unlock()
	s.goroutines.Wait()
	s.mu.Lock()

	return nil
}

// IsConnected returns whether the stream has an active client connection
func (s *Stream) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected
}

// LastDisconnectionAt returns when the stream was last disconnected
func (s *Stream) LastDisconnectionAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastDisconnectionAt
}

// ToAPI converts the stream to an API representation
func (s *Stream) ToAPI() codersdk.ImmortalStream {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stream := codersdk.ImmortalStream{
		ID:               s.id,
		Name:             s.name,
		TCPPort:          s.port,
		CreatedAt:        s.createdAt,
		LastConnectionAt: s.lastConnectionAt,
	}

	if !s.connected && !s.lastDisconnectionAt.IsZero() {
		stream.LastDisconnectionAt = &s.lastDisconnectionAt
	}

	return stream
}

// GetPipe returns the backed pipe for handling connections
func (s *Stream) GetPipe() *backedpipe.BackedPipe {
	return s.pipe
}

// startCopyingLocked starts the goroutines to copy data from local connection
// Must be called with mu held
func (s *Stream) startCopyingLocked() {
	// Copy from local connection to backed pipe
	s.goroutines.Add(1)
	go func() {
		defer s.goroutines.Done()

		_, err := io.Copy(s.pipe, s.localConn)
		if err != nil && !xerrors.Is(err, io.EOF) && !xerrors.Is(err, io.ErrClosedPipe) {
			s.logger.Debug(context.Background(), "error copying from local to pipe", slog.Error(err))
		}

		// Local connection closed, signal disconnection
		s.SignalDisconnect()
		// Don't close the pipe - it should stay alive for reconnections
	}()

	// Copy from backed pipe to local connection
	// This goroutine must continue running even when clients disconnect
	s.goroutines.Add(1)
	go func() {
		defer s.goroutines.Done()
		defer s.logger.Debug(context.Background(), "exiting copy from pipe to local goroutine")

		s.logger.Debug(context.Background(), "starting copy from pipe to local goroutine")
		// Keep copying until the stream is closed
		// The BackedPipe will block when no client is connected
		buf := make([]byte, 32*1024)
		for {
			// Use a buffer for copying
			n, err := s.pipe.Read(buf)
			// Log significant events
			if errors.Is(err, io.EOF) {
				s.logger.Debug(context.Background(), "got EOF from pipe")
				s.SignalDisconnect()
			} else if err != nil && !errors.Is(err, io.ErrClosedPipe) {
				s.logger.Debug(context.Background(), "error reading from pipe", slog.Error(err))
				s.SignalDisconnect()
			}

			if n > 0 {
				// Write to local connection
				if _, writeErr := s.localConn.Write(buf[:n]); writeErr != nil {
					s.logger.Debug(context.Background(), "error writing to local connection", slog.Error(writeErr))
					// Local connection failed, we're done
					s.SignalDisconnect()
					_ = s.localConn.Close()
					return
				}
			}

			if err != nil {
				// Check if this is a fatal error
				if xerrors.Is(err, io.ErrClosedPipe) {
					// The pipe itself is closed, we're done
					s.logger.Debug(context.Background(), "pipe closed, exiting copy goroutine")
					s.SignalDisconnect()
					return
				}
				// Any other error (including EOF) is handled by BackedPipe; continue
			}
		}
	}()

	// Start disconnection handler that listens to disconnection signals
	s.goroutines.Add(1)
	go func() {
		defer s.goroutines.Done()

		// Keep listening for disconnection signals until shutdown
		for {
			select {
			case <-s.disconnectChan:
				s.handleDisconnect()
			case <-s.shutdownChan:
				return
			}
		}
	}()
}

// handleDisconnect handles when a connection is lost
func (s *Stream) handleDisconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.connected {
		s.connected = false
		s.lastDisconnectionAt = time.Now()
		s.logger.Info(context.Background(), "stream disconnected")
	}
}

// SignalDisconnect signals that the connection has been lost
func (s *Stream) SignalDisconnect() {
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()
	if closed {
		return
	}
	select {
	case s.disconnectChan <- struct{}{}:
	default:
		// Channel is full, ignore
	}
}

// ForceDisconnect forces the stream to be marked as disconnected (for testing)
func (s *Stream) ForceDisconnect() {
	s.handleDisconnect()
}
