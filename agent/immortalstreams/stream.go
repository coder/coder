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
	"github.com/coder/coder/v2/coderd/agentapi/backedpipe"
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

	// goroutines manages the copy goroutines
	goroutines sync.WaitGroup

	// Reconnection coordination
	pendingReconnect *reconnectRequest

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
func NewStream(id uuid.UUID, name string, port int, logger slog.Logger, _ int) *Stream {
	stream := &Stream{
		id:             id,
		name:           name,
		port:           port,
		createdAt:      time.Now(),
		logger:         logger,
		disconnectChan: make(chan struct{}, 1),
		shutdownChan:   make(chan struct{}, 1),
	}

	// Create a reconnect function that waits for a client connection
	reconnectFn := func(ctx context.Context, writerSeqNum uint64) (io.ReadWriteCloser, uint64, error) {
		// Wait for HandleReconnect to be called with a new connection
		responseChan := make(chan reconnectResponse, 1)

		stream.mu.Lock()
		stream.pendingReconnect = &reconnectRequest{
			writerSeqNum: writerSeqNum,
			response:     responseChan,
		}
		stream.mu.Unlock()

		// Wait for response from HandleReconnect or context cancellation
		stream.logger.Debug(context.Background(), "reconnect function waiting for response")
		select {
		case resp := <-responseChan:
			stream.logger.Debug(context.Background(), "reconnect function got response",
				slog.F("has_conn", resp.conn != nil),
				slog.F("read_seq", resp.readSeq),
				slog.Error(resp.err))
			return resp.conn, resp.readSeq, resp.err
		case <-ctx.Done():
			// Context was canceled, clear pending request and return error
			stream.mu.Lock()
			stream.pendingReconnect = nil
			stream.mu.Unlock()
			return nil, 0, ctx.Err()
		case <-stream.shutdownChan:
			// Stream is being shut down, clear pending request and return error
			stream.mu.Lock()
			stream.pendingReconnect = nil
			stream.mu.Unlock()
			return nil, 0, xerrors.New("stream is shutting down")
		}
	}

	// Create BackedPipe with background context
	stream.pipe = backedpipe.NewBackedPipe(context.Background(), reconnectFn)

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

	// Check if BackedPipe is already waiting for a reconnection
	if s.pendingReconnect != nil {
		s.logger.Debug(context.Background(), "found pending reconnect request, responding")
		// Respond to the reconnection request
		s.pendingReconnect.response <- reconnectResponse{
			conn:    clientConn,
			readSeq: readSeqNum,
			err:     nil,
		}
		s.pendingReconnect = nil
		s.logger.Debug(context.Background(), "responded to pending reconnect request")

		// Connection will be established by the waiting goroutine
		s.lastConnectionAt = time.Now()
		s.connected = true
		s.mu.Unlock()
		s.logger.Debug(context.Background(), "client reconnection successful (pending request fulfilled)")
		return nil
	}

	// No pending request - we need to trigger a reconnection
	s.logger.Debug(context.Background(), "no pending request, will trigger reconnection")

	// Use a channel to coordinate with the reconnect function
	readyChan := make(chan struct{})
	connectDone := make(chan error, 1)

	// Prepare to intercept the next pending request
	interceptConn := clientConn
	interceptReadSeq := readSeqNum

	s.mu.Unlock()

	// Start a goroutine that will wait for the pending request and fulfill it
	go func() {
		// Signal when we're ready to intercept
		close(readyChan)

		// Poll for the pending request
		for {
			s.mu.Lock()
			if s.pendingReconnect != nil {
				// Found the pending request, fulfill it
				s.pendingReconnect.response <- reconnectResponse{
					conn:    interceptConn,
					readSeq: interceptReadSeq,
					err:     nil,
				}
				s.pendingReconnect = nil
				s.mu.Unlock()
				return
			}
			s.mu.Unlock()

			// Small sleep to avoid busy waiting
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// Wait for the interceptor to be ready
	<-readyChan

	// Now trigger the reconnection - this will call our reconnect function
	go func() {
		s.logger.Debug(context.Background(), "calling ForceReconnect")
		err := s.pipe.ForceReconnect(context.Background())
		s.logger.Debug(context.Background(), "force reconnect returned", slog.Error(err))
		connectDone <- err
	}()

	// Wait for the connection to complete
	err := <-connectDone

	s.mu.Lock()
	defer s.mu.Unlock()

	if err != nil {
		s.connected = false
		s.logger.Warn(context.Background(), "failed to connect backed pipe", slog.Error(err))
		return xerrors.Errorf("failed to establish connection: %w", err)
	}

	// Success
	s.lastConnectionAt = time.Now()
	s.connected = true
	s.logger.Debug(context.Background(), "client reconnection successful")
	return nil
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

	// Signal shutdown to any pending reconnect attempts
	select {
	case s.shutdownChan <- struct{}{}:
		// Signal sent successfully
	default:
		// Channel is full or already closed, which is fine
	}

	// Clear any pending reconnect request
	if s.pendingReconnect != nil {
		s.pendingReconnect.response <- reconnectResponse{
			conn:    nil,
			readSeq: 0,
			err:     xerrors.New("stream is shutting down"),
		}
		s.pendingReconnect = nil
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
		for {
			// Use a buffer for copying
			buf := make([]byte, 32*1024)
			n, err := s.pipe.Read(buf)
			// Log significant events
			if errors.Is(err, io.EOF) {
				s.logger.Debug(context.Background(), "got EOF from pipe, will continue")
			} else if err != nil && !errors.Is(err, io.ErrClosedPipe) {
				s.logger.Debug(context.Background(), "error reading from pipe", slog.Error(err))
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
				// Any other error (including EOF) is not fatal - the BackedPipe will handle it
				// Just continue the loop
				if !xerrors.Is(err, io.EOF) {
					s.logger.Debug(context.Background(), "non-fatal error reading from pipe, continuing", slog.Error(err))
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
	select {
	case s.disconnectChan <- struct{}{}:
	default:
		// Channel is full or closed, ignore
	}
}

// ForceDisconnect forces the stream to be marked as disconnected (for testing)
func (s *Stream) ForceDisconnect() {
	s.handleDisconnect()
}
