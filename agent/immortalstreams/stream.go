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

	// Context cancellation for BackedPipe
	cancel context.CancelFunc
}

// reconnectRequest represents a pending reconnection request
type reconnectRequest struct {
	readerSeqNum uint64
	response     chan reconnectResponse
}

// reconnectResponse represents a reconnection response
type reconnectResponse struct {
	conn    io.ReadWriteCloser
	readSeq uint64
	err     error
}

// streamReconnector implements backedpipe.Reconnector interface for Stream
type streamReconnector struct {
	s *Stream
}

// Reconnect implements the backedpipe.Reconnector interface
func (r *streamReconnector) Reconnect(ctx context.Context, readerSeqNum uint64) (io.ReadWriteCloser, uint64, error) {
	r.s.mu.Lock()

	// If there's already a pending reconnect, this is a concurrent call.
	// We should return an error to let the BackedPipe retry later.
	if r.s.pendingReconnect != nil {
		r.s.mu.Unlock()
		return nil, 0, xerrors.New("reconnection already in progress")
	}

	// Fast path: if the stream is already shutting down, abort immediately
	if r.s.closed {
		r.s.mu.Unlock()
		return nil, 0, xerrors.New("stream is shutting down")
	}

	// Wait for HandleReconnect to be called with a new connection
	responseChan := make(chan reconnectResponse, 1)
	r.s.pendingReconnect = &reconnectRequest{
		readerSeqNum: readerSeqNum,
		response:     responseChan,
	}
	r.s.handshakePending = true
	// Mark disconnected if we previously had a client connection
	if r.s.connected {
		r.s.connected = false
		r.s.lastDisconnectionAt = time.Now()
	}
	r.s.logger.Debug(context.Background(), "pending reconnect set",
		slog.F("reader_seq", readerSeqNum))
	// Signal waiters a reconnect request is pending
	r.s.reconnectCond.Broadcast()
	r.s.mu.Unlock()

	// Wait for response from HandleReconnect or context cancellation with timeout
	r.s.logger.Debug(context.Background(), "reconnect function waiting for response")

	// Add a timeout to prevent indefinite hanging
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

	select {
	case resp := <-responseChan:
		r.s.logger.Debug(context.Background(), "reconnect function got response",
			slog.F("has_conn", resp.conn != nil),
			slog.F("read_seq", resp.readSeq),
			slog.Error(resp.err))
		return resp.conn, resp.readSeq, resp.err
	case <-ctx.Done():
		// Context was canceled, return error immediately
		// The stream's Close() method will handle cleanup
		r.s.logger.Debug(context.Background(), "reconnect function context canceled", slog.Error(ctx.Err()))
		return nil, 0, ctx.Err()
	case <-r.s.shutdownChan:
		// Stream is being shut down, return error immediately
		// The stream's Close() method will handle cleanup
		r.s.logger.Debug(context.Background(), "reconnect function shutdown signal received")
		return nil, 0, xerrors.New("stream is shutting down")
	case <-timeout.C:
		// Timeout occurred - clean up the pending request
		r.s.mu.Lock()
		if r.s.pendingReconnect != nil {
			r.s.pendingReconnect = nil
			r.s.handshakePending = false
		}
		r.s.mu.Unlock()
		r.s.logger.Debug(context.Background(), "reconnect function timed out")
		return nil, 0, xerrors.New("timeout waiting for reconnection response")
	}
}

// NewStream creates a new immortal stream
func NewStream(id uuid.UUID, name string, port int, logger slog.Logger) *Stream {
	// Create a context that will be canceled when the stream is closed
	ctx, cancel := context.WithCancel(context.Background())

	stream := &Stream{
		id:             id,
		name:           name,
		port:           port,
		createdAt:      time.Now(),
		logger:         logger,
		disconnectChan: make(chan struct{}, 1),
		shutdownChan:   make(chan struct{}),
		reconnectReq:   make(chan struct{}, 1),
		cancel:         cancel, // Store cancel function for cleanup
	}
	stream.reconnectCond = sync.NewCond(&stream.mu)

	// Create BackedPipe with streamReconnector
	reconnector := &streamReconnector{s: stream}
	stream.pipe = backedpipe.NewBackedPipe(ctx, reconnector)

	// Start reconnect worker: dedupe pokes and call ForceReconnect when safe.
	go func() {
		for {
			select {
			case <-stream.shutdownChan:
				return
			case <-stream.reconnectReq:
				// Drain extra pokes
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
				streamDisconnected := !stream.connected
				pipeDisconnected := stream.pipe != nil && !stream.pipe.Connected()
				// Can reconnect if either the stream OR the pipe is disconnected
				canReconnect := stream.pipe != nil && (streamDisconnected || pipeDisconnected)
				stream.mu.Unlock()
				if closed || handshaking || !canReconnect {
					// Nothing to do now; wait for a future poke.
					continue
				}
				// BackedPipe handles singleflight internally.
				_ = stream.pipe.ForceReconnect()
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

	s.logger.Debug(context.Background(), "handling reconnection",
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

			// The connection has been provided to the BackedPipe via the response channel.
			// The BackedPipe will establish the connection, and since we control the
			// reconnection process, we know it will succeed (or the Reconnect method
			// would have returned an error).
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

		// If already connected, wait for a reconnect slot instead of immediately
		// rejecting this connection. This avoids client-side reconnect storms
		// when a new connection races with the server observing the prior
		// connection loss.
		if s.connected {
			s.logger.Debug(context.Background(), "already connected; waiting for reconnect slot")
			// Ensure a reconnect attempt is requested while we wait.
			requestReconnect()
			// Wait until state changes: pendingReconnect set, connection released, or closed.
			s.reconnectCond.Wait()
			// Re-check loop conditions under lock.
			continue
		}

		// Ensure a reconnect attempt is requested while we wait.
		requestReconnect()

		// Wait until state changes: pendingReconnect set, connection established, or closed.
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

	// Cancel will interrupt any pending BackedPipe operations
	if s.cancel != nil {
		s.cancel()
	}

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

	// Get references to resources we need to close, but close them outside the mutex
	// to avoid deadlocks with reconnection attempts
	pipe := s.pipe
	localConn := s.localConn

	// Release the mutex before closing resources to avoid deadlocks
	s.mu.Unlock()

	// Close the backed pipe (this can trigger reconnection attempts, so must be outside mutex)
	if pipe != nil {
		if err := pipe.Close(); err != nil {
			s.logger.Warn(context.Background(), "failed to close backed pipe", slog.Error(err))
		}
	}

	// Close connections
	if localConn != nil {
		if err := localConn.Close(); err != nil {
			s.logger.Warn(context.Background(), "failed to close local connection", slog.Error(err))
		}
	}

	// Wait for goroutines to finish
	s.goroutines.Wait()

	// Re-acquire mutex for final cleanup and clear the references
	s.mu.Lock()
	s.pipe = nil
	s.localConn = nil

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
			// Check if we should shut down before attempting to read
			select {
			case <-s.shutdownChan:
				s.logger.Debug(context.Background(), "shutdown signal received, exiting copy goroutine")
				return
			default:
			}

			// Use a buffer for copying
			n, err := s.pipe.Read(buf)
			if err != nil {
				// Check for fatal errors that should terminate the goroutine
				if xerrors.Is(err, io.ErrClosedPipe) {
					// The pipe itself is closed, we're done
					s.logger.Debug(context.Background(), "pipe closed, exiting copy goroutine")
					s.SignalDisconnect()
					return
				}

				// Check for BackedPipe specific errors
				if xerrors.Is(err, backedpipe.ErrPipeClosed) {
					s.logger.Debug(context.Background(), "backed pipe closed, exiting copy goroutine")
					s.SignalDisconnect()
					return
				}

				// Log other errors but continue
				if errors.Is(err, io.EOF) {
					s.logger.Debug(context.Background(), "got EOF from pipe")
					s.SignalDisconnect()
				} else {
					s.logger.Debug(context.Background(), "error reading from pipe", slog.Error(err))
					s.SignalDisconnect()
				}

				// For non-fatal errors, continue the loop
				continue
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
	// Also signal disconnection to trigger proper cleanup and reconnection readiness
	s.SignalDisconnect()
}
