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
	port      uint16
	createdAt time.Time
	logger    slog.Logger

	mu                  sync.RWMutex
	localConn           io.ReadWriteCloser
	pipe                *backedpipe.BackedPipe
	lastConnectionAt    time.Time
	lastDisconnectionAt time.Time
	closed              bool

	// goroutines manages the copy goroutines
	goroutines sync.WaitGroup

	// Indicates whether the upstream (local -> pipe) copy goroutine has been started.
	upstreamCopyStarted bool

	// Disconnection detection
	disconnectChan chan struct{}

	// Shutdown signal
	shutdownChan chan struct{}

	// Context cancellation for BackedPipe
	cancel context.CancelFunc
}

// NewStream creates a new immortal stream
func NewStream(id uuid.UUID, name string, port uint16, logger slog.Logger) *Stream {
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
		cancel:         cancel, // Store cancel function for cleanup
		// Create BackedPipe without a reconnector; reconnections are accepted
		// explicitly via HandleReconnect.
		pipe: backedpipe.NewBackedPipe(ctx, nil),
	}

	return stream
}

// setNameAndLogger sets the stream name and updates the logger to include it.
// Must be called by the manager under its own lock before publishing the stream.
func (s *Stream) setNameAndLogger(name string, baseLogger slog.Logger) {
	s.mu.Lock()
	s.name = name
	s.logger = baseLogger.With(slog.F("stream_name", name))
	s.mu.Unlock()
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

	// Start copying data between the local connection and the backed pipe
	s.startCopyingLocked()

	return nil
}

// HandleReconnect handles a client reconnection
func (s *Stream) HandleReconnect(clientConn io.ReadWriteCloser, readSeqNum uint64) error {
	// Fast-path check: ensure the stream isn't closed, then operate on the
	// backed pipe without holding the stream mutex.
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return xerrors.New("stream is closed")
	}
	s.mu.RUnlock()

	// Attach the new connection and replay outbound data from the client's
	// acknowledged sequence number.
	if err := s.pipe.AcceptReconnection(readSeqNum, clientConn); err != nil {
		_ = clientConn.Close()
		return xerrors.Errorf("accept reconnection: %w", err)
	}

	// Update state
	s.mu.Lock()
	s.lastConnectionAt = time.Now()
	// Start upstream copy lazily on first client connection
	if !s.upstreamCopyStarted {
		s.upstreamCopyStarted = true
		s.goroutines.Add(1)
		local := s.localConn
		p := s.pipe
		s.mu.Unlock()
		go func() {
			defer s.goroutines.Done()
			if local == nil || p == nil {
				return
			}
			_, err := io.Copy(p, local)
			if err != nil && !xerrors.Is(err, io.EOF) && !xerrors.Is(err, io.ErrClosedPipe) {
				s.logger.Debug(context.Background(), "error copying from local to pipe", slog.Error(err))
			}
			s.SignalDisconnect()
		}()
	} else {
		s.mu.Unlock()
	}

	s.logger.Debug(context.Background(), "client reconnection successful",
		slog.F("read_seq_num", readSeqNum))
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

	// Cancel will interrupt any pending BackedPipe operations
	if s.cancel != nil {
		s.cancel()
	}

	// Signal shutdown to any pending reconnect attempts and listeners
	// Closing the channel wakes all waiters exactly once
	close(s.shutdownChan)

	// No reconnection waiters in the simplified model.

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

	// Re-acquire mutex to balance the deferred unlock
	s.mu.Lock()
	return nil
}

// IsConnected returns whether the stream has an active client connection
func (s *Stream) IsConnected() bool {
	p := s.pipe
	if p == nil {
		return false
	}
	return p.Connected()
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

	if !s.IsConnected() && !s.lastDisconnectionAt.IsZero() {
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
	// Defer starting upstream (local -> pipe) copying until we have a client attached.
	// This reduces load and eliminates a large number of blocked goroutines in tests
	// that create many streams without immediate clients.
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
					// Keep the goroutine alive to handle future reconnections
					continue
				}

				// Check for BackedPipe specific errors
				if xerrors.Is(err, backedpipe.ErrPipeClosed) {
					s.logger.Debug(context.Background(), "backed pipe closed, exiting copy goroutine")
					s.SignalDisconnect()
					// Keep the goroutine alive to handle future reconnections
					continue
				}

				// Treat EOF as terminal: the pipe is closed and this goroutine should exit
				if errors.Is(err, io.EOF) {
					s.logger.Debug(context.Background(), "got EOF from pipe, waiting for reconnection")
					s.SignalDisconnect()
					// Keep the goroutine alive to handle future reconnections
					continue
				}

				// Log other errors but continue (reconnect will eventually succeed)
				{
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

	s.lastDisconnectionAt = time.Now()
	s.logger.Info(context.Background(), "stream disconnected")
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
