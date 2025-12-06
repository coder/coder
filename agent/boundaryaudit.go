package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/proto"
)

const (
	// BoundaryAuditSocketName is the name of the Unix socket file that Boundary
	// connects to for reporting network audit events.
	BoundaryAuditSocketName = "coder-boundary-audit.sock"
)

// BoundaryAuditEvent represents a single resource access event from Boundary.
// This matches the JSON format that Boundary sends.
type BoundaryAuditEvent struct {
	Timestamp    time.Time `json:"timestamp"`
	ResourceType string    `json:"resource_type"` // "network", "file", etc.
	Resource     string    `json:"resource"`      // URL, file path, etc.
	Operation    string    `json:"operation"`     // "GET", "POST", "read", "write", etc.
	Decision     string    `json:"decision"`      // "allow" or "deny"
}

// BoundaryAuditReporter is the interface for reporting boundary network audit logs to coderd.
type BoundaryAuditReporter interface {
	ReportBoundaryAuditLogs(ctx context.Context, req *proto.ReportBoundaryAuditLogsRequest) (*emptypb.Empty, error)
}

// BoundaryAuditListenerConfig holds configuration for BoundaryAuditListener.
type BoundaryAuditListenerConfig struct {
	Logger   slog.Logger
	SockDir  string
	Reporter BoundaryAuditReporter // Optional: for sending to coderd
	// OTEL configuration
	OTELEndpoint string            // If set, logs are sent to OTEL
	OTELHeaders  map[string]string // Optional headers for OTEL endpoint
	SendToCoderd bool              // If true, also send to coderd when OTEL is enabled
}

// BoundaryAuditListener listens on a Unix socket for network audit events from
// Boundary and forwards them to OTEL and/or coderd.
type BoundaryAuditListener struct {
	logger       slog.Logger
	sockDir      string
	reporter     BoundaryAuditReporter
	otelExporter *OTELExporter
	sendToCoderd bool

	mu       sync.Mutex
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewBoundaryAuditListener creates a new boundary audit listener.
func NewBoundaryAuditListener(config BoundaryAuditListenerConfig) (*BoundaryAuditListener, error) {
	l := &BoundaryAuditListener{
		logger:       config.Logger.Named("boundary-audit"),
		sockDir:      config.SockDir,
		reporter:     config.Reporter,
		sendToCoderd: config.SendToCoderd,
	}

	// Initialize OTEL exporter if endpoint is configured.
	if config.OTELEndpoint != "" {
		exporter, err := NewOTELExporter(OTELExporterConfig{
			Logger:   config.Logger,
			Endpoint: config.OTELEndpoint,
			Headers:  config.OTELHeaders,
		})
		if err != nil {
			return nil, xerrors.Errorf("create OTEL exporter: %w", err)
		}
		l.otelExporter = exporter
		l.logger.Info(context.Background(), "OTEL export enabled for boundary audit logs",
			slog.F("endpoint", config.OTELEndpoint))
	}

	// If no OTEL endpoint, default to sending to coderd.
	if l.otelExporter == nil {
		l.sendToCoderd = true
	}

	return l, nil
}

// HasOTELExport returns true if OTEL export is enabled.
func (l *BoundaryAuditListener) HasOTELExport() bool {
	return l.otelExporter != nil
}

// SendsToCoderd returns true if logs are sent to coderd.
func (l *BoundaryAuditListener) SendsToCoderd() bool {
	return l.sendToCoderd
}

// SocketPath returns the full path to the Unix socket.
func (l *BoundaryAuditListener) SocketPath() string {
	return filepath.Join(l.sockDir, BoundaryAuditSocketName)
}

// Start begins listening for connections on the Unix socket.
func (l *BoundaryAuditListener) Start(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.listener != nil {
		return xerrors.New("listener already started")
	}

	socketPath := l.SocketPath()

	// Remove any existing socket file.
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return xerrors.Errorf("remove existing socket: %w", err)
	}

	// Ensure the directory exists.
	if err := os.MkdirAll(l.sockDir, 0o700); err != nil {
		return xerrors.Errorf("create socket directory: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return xerrors.Errorf("listen on socket: %w", err)
	}

	// Make socket accessible.
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = listener.Close()
		return xerrors.Errorf("chmod socket: %w", err)
	}

	l.listener = listener
	l.ctx, l.cancel = context.WithCancel(ctx)

	l.logger.Info(l.ctx, "boundary audit listener started", slog.F("path", socketPath))

	// Start accepting connections.
	l.wg.Add(1)
	go l.acceptLoop()

	return nil
}

// Close stops the listener and closes all connections.
func (l *BoundaryAuditListener) Close() error {
	l.mu.Lock()
	if l.cancel != nil {
		l.cancel()
	}
	if l.listener != nil {
		_ = l.listener.Close()
	}
	l.mu.Unlock()

	l.wg.Wait()

	// Close OTEL exporter if configured.
	if l.otelExporter != nil {
		if err := l.otelExporter.Close(); err != nil {
			l.logger.Warn(context.Background(), "failed to close OTEL exporter", slog.Error(err))
		}
	}

	// Clean up socket file.
	_ = os.Remove(l.SocketPath())

	return nil
}

func (l *BoundaryAuditListener) acceptLoop() {
	defer l.wg.Done()

	for {
		conn, err := l.listener.Accept()
		if err != nil {
			select {
			case <-l.ctx.Done():
				return
			default:
				l.logger.Warn(l.ctx, "failed to accept connection", slog.Error(err))
				continue
			}
		}

		l.wg.Add(1)
		go l.handleConnection(conn)
	}
}

func (l *BoundaryAuditListener) handleConnection(conn net.Conn) {
	defer l.wg.Done()
	defer conn.Close()

	l.logger.Debug(l.ctx, "boundary connected")

	scanner := bufio.NewScanner(conn)
	// Increase buffer size for potentially large batches.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-l.ctx.Done():
			return
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Parse the batch of events (JSON array).
		var events []BoundaryAuditEvent
		if err := json.Unmarshal(line, &events); err != nil {
			l.logger.Warn(l.ctx, "failed to parse audit events", slog.Error(err), slog.F("line", string(line)))
			continue
		}

		if len(events) == 0 {
			continue
		}

		// Export to OTEL if configured.
		if l.otelExporter != nil {
			l.otelExporter.Export(events)
		}

		// Forward to coderd if configured.
		if l.sendToCoderd && l.reporter != nil {
			// Convert to proto format.
			protoLogs := make([]*proto.BoundaryAuditLog, len(events))
			for i, event := range events {
				protoLogs[i] = &proto.BoundaryAuditLog{
					Timestamp:    timestamppb.New(event.Timestamp),
					ResourceType: event.ResourceType,
					Resource:     event.Resource,
					Operation:    event.Operation,
					Decision:     event.Decision,
				}
			}

			// Forward to coderd (fire-and-forget with error logging).
			_, err := l.reporter.ReportBoundaryAuditLogs(l.ctx, &proto.ReportBoundaryAuditLogsRequest{
				Logs: protoLogs,
			})
			if err != nil {
				l.logger.Warn(l.ctx, "failed to report audit logs to coderd", slog.F("error_string", err.Error()), slog.F("count", len(events)))
			} else {
				l.logger.Debug(l.ctx, "reported audit logs to coderd", slog.F("count", len(events)))
			}
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case <-l.ctx.Done():
			// Expected during shutdown.
		default:
			l.logger.Warn(l.ctx, "scanner error", slog.Error(err))
		}
	}
}
