package immortalstreams

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
)

// Package-level sentinel errors
var (
	ErrTooManyStreams   = xerrors.New("too many streams")
	ErrStreamNotFound   = xerrors.New("stream not found")
	ErrConnRefused      = xerrors.New("connection refused")
	ErrAlreadyConnected = xerrors.New("already connected")
)

const (
	// MaxStreams is the maximum number of immortal streams allowed per agent
	MaxStreams = 32
)

// Manager manages immortal streams for an agent
type Manager struct {
	logger slog.Logger

	mu      sync.RWMutex
	streams map[uuid.UUID]*Stream

	// dialer is used to dial services
	dialer Dialer
}

// Dialer dials a local service
type Dialer interface {
	DialContext(ctx context.Context, address string) (net.Conn, error)
}

// New creates a new immortal streams manager
func New(logger slog.Logger, dialer Dialer) *Manager {
	return &Manager{
		logger:  logger,
		streams: make(map[uuid.UUID]*Stream),
		dialer:  dialer,
	}
}

// CreateStream creates a new immortal stream
func (m *Manager) CreateStream(ctx context.Context, port int) (*codersdk.ImmortalStream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if we're at the limit
	if len(m.streams) >= MaxStreams {
		// Try to evict a disconnected stream
		evicted := m.evictOldestDisconnectedLocked()
		if !evicted {
			return nil, ErrTooManyStreams
		}
	}

	// Always dial localhost; internal listeners are handled by the dialer.
	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := m.dialer.DialContext(ctx, addr)
	if err != nil {
		if isConnectionRefused(err) {
			return nil, ErrConnRefused
		}
		return nil, xerrors.Errorf("dial local service: %w", err)
	}

	// Create the stream
	id := uuid.New()
	name := namesgenerator.GetRandomName(0)
	stream := NewStream(
		id,
		name,
		port,
		m.logger.With(slog.F("stream_id", id), slog.F("stream_name", name)),
	)

	// Start the stream
	if err := stream.Start(conn); err != nil {
		_ = conn.Close()
		return nil, xerrors.Errorf("start stream: %w", err)
	}

	m.streams[id] = stream

	// Return the API representation of the stream
	apiStream := stream.ToAPI()
	return &apiStream, nil
}

// GetStream returns a stream by ID
func (m *Manager) GetStream(id uuid.UUID) (*Stream, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stream, ok := m.streams[id]
	return stream, ok
}

// ListStreams returns all streams
func (m *Manager) ListStreams() []codersdk.ImmortalStream {
	m.mu.RLock()
	defer m.mu.RUnlock()

	streams := make([]codersdk.ImmortalStream, 0, len(m.streams))
	for _, stream := range m.streams {
		streams = append(streams, stream.ToAPI())
	}
	return streams
}

// DeleteStream deletes a stream by ID
func (m *Manager) DeleteStream(id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	stream, ok := m.streams[id]
	if !ok {
		return ErrStreamNotFound
	}

	if err := stream.Close(); err != nil {
		m.logger.Warn(context.Background(), "failed to close stream", slog.Error(err))
	}

	delete(m.streams, id)
	return nil
}

// Close closes all streams
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for id, stream := range m.streams {
		if err := stream.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(m.streams, id)
	}
	return firstErr
}

// evictOldestDisconnectedLocked evicts the oldest disconnected stream
// Must be called with mu held
func (m *Manager) evictOldestDisconnectedLocked() bool {
	var (
		oldestID           uuid.UUID
		oldestDisconnected time.Time
		found              bool
	)

	for id, stream := range m.streams {
		if stream.IsConnected() {
			continue
		}

		disconnectedAt := stream.LastDisconnectionAt()

		// Prioritize streams that have actually been disconnected over never-connected streams
		switch {
		case !found:
			oldestID = id
			oldestDisconnected = disconnectedAt
			found = true
		case disconnectedAt.IsZero() && !oldestDisconnected.IsZero():
			// Keep the current choice (it was actually disconnected)
			continue
		case !disconnectedAt.IsZero() && oldestDisconnected.IsZero():
			// Prefer this stream (it was actually disconnected) over never-connected
			oldestID = id
			oldestDisconnected = disconnectedAt
		case !disconnectedAt.IsZero() && !oldestDisconnected.IsZero():
			// Both were actually disconnected, pick the oldest
			if disconnectedAt.Before(oldestDisconnected) {
				oldestID = id
				oldestDisconnected = disconnectedAt
			}
		}
		// If both are zero time, keep the first one found
	}

	if !found {
		return false
	}

	// Close and remove the oldest disconnected stream
	if stream, ok := m.streams[oldestID]; ok {
		m.logger.Info(context.Background(), "evicting oldest disconnected stream",
			slog.F("stream_id", oldestID),
			slog.F("stream_name", stream.name),
			slog.F("disconnected_at", oldestDisconnected))

		if err := stream.Close(); err != nil {
			m.logger.Warn(context.Background(), "failed to close evicted stream", slog.Error(err))
		}
		delete(m.streams, oldestID)
	}

	return true
}

// HandleConnection handles a new connection for an existing stream
func (m *Manager) HandleConnection(id uuid.UUID, conn io.ReadWriteCloser, readSeqNum uint64) error {
	m.mu.RLock()
	stream, ok := m.streams[id]
	m.mu.RUnlock()

	if !ok {
		return ErrStreamNotFound
	}

	return stream.HandleReconnect(conn, readSeqNum)
}

// isConnectionRefused checks if an error is a connection refused error
func isConnectionRefused(err error) bool {
	// Check for syscall.ECONNREFUSED through error unwrapping
	var errno syscall.Errno
	if errors.As(err, &errno) && errno == syscall.ECONNREFUSED {
		return true
	}

	// Fallback: check for net.OpError with "dial" operation
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		// Check if the underlying error is ECONNREFUSED
		if errors.As(opErr.Err, &errno) && errno == syscall.ECONNREFUSED {
			return true
		}
	}

	// Cross-platform fallback: check error message for common connection refused patterns
	// This handles Windows (connectex) and other platforms that might have different error constants
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connectex: No connection could be made because the target machine actively refused it") ||
		strings.Contains(errStr, "actively refused")
}
