// Package agentsdk provides boundary log forwarding utilities.
package agentsdk

import (
	"encoding/binary"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	agentproto "github.com/coder/coder/v2/agent/proto"
)

const (
	boundaryLogFlushInterval = 5 * time.Second
	boundaryLogBatchSize     = 10
)

// BoundaryAuditor implements the boundary audit.Auditor interface by sending
// logs to the agent's boundary log proxy socket. It batches logs using either
// a 5-second timeout or when 10 logs have accumulated, whichever comes first.
type BoundaryAuditor struct {
	socketPath  string
	workspaceID uuid.UUID

	mu     sync.Mutex
	conn   net.Conn
	logs   []*agentproto.BoundaryLog
	closed bool

	// For batching
	flushTimer *time.Timer
}

// NewBoundaryAuditor creates a new BoundaryAuditor that sends logs to the
// agent's boundary log proxy socket.
func NewBoundaryAuditor(socketPath string, workspaceID uuid.UUID) *BoundaryAuditor {
	return &BoundaryAuditor{
		socketPath:  socketPath,
		workspaceID: workspaceID,
	}
}

// AuditRequest implements the audit.Auditor interface from the boundary package.
// It queues the request and batches sends to the agent socket.
func (b *BoundaryAuditor) AuditRequest(method, url string, allowed bool, matchedRule string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	log := &agentproto.BoundaryLog{
		WorkspaceId: b.workspaceID[:],
		Time:        timestamppb.Now(),
		Allowed:     allowed,
		HttpMethod:  method,
		HttpUrl:     url,
		MatchedRule: matchedRule,
	}
	b.logs = append(b.logs, log)

	// Start flush timer if this is the first log.
	if len(b.logs) == 1 {
		b.flushTimer = time.AfterFunc(boundaryLogFlushInterval, func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.flushLocked()
		})
	}

	// Flush immediately if we've reached batch size.
	if len(b.logs) >= boundaryLogBatchSize {
		if b.flushTimer != nil {
			b.flushTimer.Stop()
			b.flushTimer = nil
		}
		b.flushLocked()
	}
}

// flushLocked sends the current batch of logs. Caller must hold the lock.
func (b *BoundaryAuditor) flushLocked() {
	if len(b.logs) == 0 {
		return
	}

	// Try to connect if not connected.
	if b.conn == nil {
		conn, err := net.Dial("unix", b.socketPath)
		if err != nil {
			// Drop logs if we can't connect.
			b.logs = nil
			return
		}
		b.conn = conn
	}

	req := &agentproto.ReportBoundaryLogsRequest{
		Logs: b.logs,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		b.logs = nil
		return
	}

	// Write length-prefixed message.
	if err := binary.Write(b.conn, binary.BigEndian, uint32(len(data))); err != nil {
		// Connection error - close and try again next time.
		_ = b.conn.Close()
		b.conn = nil
		b.logs = nil
		return
	}
	if _, err := b.conn.Write(data); err != nil {
		_ = b.conn.Close()
		b.conn = nil
		b.logs = nil
		return
	}

	b.logs = nil
}

// Close flushes any remaining logs and closes the connection.
func (b *BoundaryAuditor) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	if b.flushTimer != nil {
		b.flushTimer.Stop()
		b.flushTimer = nil
	}

	b.flushLocked()

	if b.conn != nil {
		err := b.conn.Close()
		b.conn = nil
		return err
	}
	return nil
}

// SocketPath returns the path to the boundary log socket.
// This is typically provided via the CODER_BOUNDARY_LOG_SOCKET environment variable.
func BoundaryLogSocketPath() string {
	// The agent sets this environment variable when starting boundary.
	return "" // Will be set by the agent via environment variable
}

// ErrSocketNotConfigured is returned when the boundary log socket is not configured.
var ErrSocketNotConfigured = xerrors.New("boundary log socket not configured")
