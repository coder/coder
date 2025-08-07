package immortalstreams

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// MaxStreams is the maximum number of immortal streams allowed per agent
	MaxStreams = 32
	// BufferSize is the size of the ring buffer for each stream (64 MiB)
	BufferSize = 64 * 1024 * 1024
)

// Manager manages immortal streams for an agent
type Manager struct {
	logger slog.Logger

	mu      sync.RWMutex
	streams map[uuid.UUID]*Stream

	// dialer is used to dial local services
	dialer Dialer
}

// Dialer dials a local service
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
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
			return nil, xerrors.New("too many immortal streams")
		}
	}

	// Dial the local service
	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := m.dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		if isConnectionRefused(err) {
			return nil, xerrors.Errorf("the connection was refused")
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
		BufferSize,
	)

	// Start the stream
	if err := stream.Start(conn); err != nil {
		_ = conn.Close()
		return nil, xerrors.Errorf("start stream: %w", err)
	}

	m.streams[id] = stream

	return &codersdk.ImmortalStream{
		ID:               id,
		Name:             name,
		TCPPort:          port,
		CreatedAt:        stream.createdAt,
		LastConnectionAt: stream.createdAt,
	}, nil
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
		return xerrors.New("stream not found")
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
		return xerrors.New("stream not found")
	}

	return stream.HandleReconnect(conn, readSeqNum)
}

// isConnectionRefused checks if an error is a connection refused error
func isConnectionRefused(err error) bool {
	var opErr *net.OpError
	if xerrors.As(err, &opErr) {
		return opErr.Op == "dial"
	}
	return false
}
