package immortalstreams

import (
	"context"
	"errors"
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

// Dialer dials a local service by TCP port
type Dialer interface {
	DialPort(ctx context.Context, port uint16) (net.Conn, error)
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
func (m *Manager) CreateStream(ctx context.Context, port uint16) (*codersdk.ImmortalStream, error) {
	// Always dial by port; internal listeners are handled by the dialer.
	conn, err := m.dialer.DialPort(ctx, port)
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

	// Start the stream outside of the manager lock to avoid blocking other operations.
	if err := stream.Start(conn); err != nil {
		_ = conn.Close()
		return nil, xerrors.Errorf("start stream: %w", err)
	}

	// Insert the stream, enforcing capacity without holding the lock during dial/start.
	for {
		m.mu.Lock()
		if len(m.streams) < MaxStreams {
			m.streams[id] = stream
			m.mu.Unlock()
			break
		}
		m.mu.Unlock()
		// Try to evict a disconnected stream without holding the manager lock while closing it.
		if !m.evictOldestDisconnected() {
			// No space and nothing to evict: close the newly started stream and return an error.
			_ = stream.Close()
			return nil, ErrTooManyStreams
		}
		// Loop and try insertion again after eviction.
	}

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
	stream, ok := m.streams[id]
	if !ok {
		m.mu.Unlock()
		return ErrStreamNotFound
	}
	delete(m.streams, id)
	m.mu.Unlock()

	// Close outside the manager lock to avoid blocking other operations.
	if err := stream.Close(); err != nil {
		m.logger.Warn(context.Background(), "failed to close stream", slog.Error(err))
	}

	return nil
}

// Close closes all streams
func (m *Manager) Close() error {
	m.mu.Lock()
	// Move streams out of the map so we can close them without holding the lock.
	streams := make([]*Stream, 0, len(m.streams))
	for id, stream := range m.streams {
		streams = append(streams, stream)
		delete(m.streams, id)
	}
	m.mu.Unlock()

	var firstErr error
	for _, stream := range streams {
		if err := stream.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// evictOldestDisconnected finds, removes, and closes the oldest disconnected stream.
// "Oldest" is computed using the max(created_at, last_disconnection_at) for disconnected streams.
// If all streams are currently connected, returns false without evicting.
// Closing happens outside of the manager lock to avoid blocking other operations.
func (m *Manager) evictOldestDisconnected() bool {
	var (
		oldestID         uuid.UUID
		oldestActivityAt time.Time
		found            bool
		toClose          *Stream
	)

	// Find and remove the candidate under lock
	m.mu.Lock()
	for id, stream := range m.streams {
		if stream.IsConnected() {
			continue
		}

		// Compute activityAt = max(createdAt, lastDisconnectionAt) for eviction ordering
		disconnectedAt := stream.LastDisconnectionAt()
		activityAt := disconnectedAt
		if activityAt.IsZero() || stream.createdAt.After(activityAt) {
			activityAt = stream.createdAt
		}

		if !found || activityAt.Before(oldestActivityAt) {
			oldestID = id
			oldestActivityAt = activityAt
			found = true
		}
	}

	if !found {
		m.mu.Unlock()
		return false
	}

	toClose = m.streams[oldestID]
	delete(m.streams, oldestID)
	m.mu.Unlock()

	if toClose != nil {
		m.logger.Info(context.Background(), "evicting oldest disconnected stream",
			slog.F("stream_id", oldestID),
			slog.F("stream_name", toClose.name),
			slog.F("eviction_activity_at", oldestActivityAt))

		if err := toClose.Close(); err != nil {
			m.logger.Warn(context.Background(), "failed to close evicted stream", slog.Error(err))
		}
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
