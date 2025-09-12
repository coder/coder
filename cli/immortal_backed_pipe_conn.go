package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/immortalstreams/backedpipe"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/websocket"
)

// immortalBackedConn adapts a BackedPipe to net.Conn for client-side immortal streams.
type immortalBackedConn struct {
	ctx    context.Context
	cancel context.CancelFunc

	pipe   *backedpipe.BackedPipe
	logger slog.Logger
}

// clientStreamReconnector dials the agent websocket and exchanges sequence numbers.
type clientStreamReconnector struct {
	mu        sync.RWMutex
	agentConn workspacesdk.AgentConn
	client    *codersdk.Client
	agentID   uuid.UUID
	dialOpts  *workspacesdk.DialAgentOptions
	streamID  uuid.UUID
	logger    slog.Logger
}

func (r *clientStreamReconnector) Reconnect(ctx context.Context, readerSeqNum uint64) (io.ReadWriteCloser, uint64, error) {
	// Build URL to agent HTTP API on localhost inside tailnet
	apiAddr := fmt.Sprintf("127.0.0.1:%d", workspacesdk.AgentHTTPAPIServerPort)
	wsURL := fmt.Sprintf("ws://%s/api/v0/immortal-stream/%s", apiAddr, r.streamID)

	// Prepare dial options using agentConn for transport. Always fetch the
	// latest agentConn under lock to support live refresh.
	dialOptions := &websocket.DialOptions{
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
					r.logger.Debug(context.Background(), "dialing network connection", slog.F("network", network), slog.F("addr", addr))
					ac := r.getAgentConn()
					return ac.DialContext(dialCtx, network, addr)
				},
			},
		},
		HTTPHeader: http.Header{
			codersdk.HeaderImmortalStreamSequenceNum: []string{strconv.FormatUint(readerSeqNum, 10)},
		},
		CompressionMode: websocket.CompressionDisabled,
	}

	// Per-attempt timeout: keep reconnect attempts snappy
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// If the underlying tailnet has been closed, refresh before dialing.
	if ac := r.getAgentConn(); ac != nil {
		select {
		case <-ac.TailnetConn().Closed():
			r.logger.Warn(ctx, "agent tailnet connection closed, refreshing before dial", slog.F("url", wsURL))
			if err := r.refreshAgentConn(dialCtx); err != nil {
				r.logger.Error(ctx, "failed to refresh agent connection before dial", slog.Error(err))
				// continue and let the dial below fail; supervisor will retry
			}
		default:
		}
	}

	r.logger.Debug(ctx, "immortal reconnect dialing", slog.F("url", wsURL), slog.F("reader_seq", readerSeqNum))
	ws, resp, err := websocket.Dial(dialCtx, wsURL, dialOptions)
	if err != nil {
		// Decide if we should refresh the underlying AgentConn and retry once.
		if r.shouldRefreshOnDialError(resp, err) {
			r.logger.Warn(ctx, "dial failed; attempting to refresh agent connection", slog.Error(err))
			// Use a fresh timeout context for the refresh and the subsequent retry
			refreshCtx, refreshCancel := context.WithTimeout(ctx, 2*time.Second)
			defer refreshCancel()
			if rErr := r.refreshAgentConn(refreshCtx); rErr == nil {
				// Extra guard: ensure the new agent connection reports reachability
				if ac := r.getAgentConn(); ac != nil {
					reachCtx, reachCancel := context.WithTimeout(ctx, 2*time.Second)
					reachable := ac.AwaitReachable(reachCtx)
					reachCancel()
					r.logger.Debug(ctx, "post-refresh reachability check", slog.F("reachable", reachable))
				}
				// Retry handshake with a new 2s timeout separate from the original dialCtx
				retryCtx, retryCancel := context.WithTimeout(ctx, 2*time.Second)
				ws, resp, err = websocket.Dial(retryCtx, wsURL, dialOptions)
				retryCancel()
			}
		}

		if err != nil {
			var status string
			var hdr http.Header
			var bodyStr string
			if resp != nil {
				status = resp.Status
				hdr = resp.Header.Clone()
				if resp.Body != nil {
					b, _ := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if len(b) > 1024 {
						b = b[:1024]
					}
					bodyStr = string(b)
				}
			}
			r.logger.Error(ctx, "immortal reconnect dial failed", slog.Error(err), slog.F("url", wsURL), slog.F("status", status), slog.F("headers", hdr), slog.F("body", bodyStr))
			return nil, 0, xerrors.Errorf("failed to WebSocket dial: %w", err)
		}
	}

	// Get remote reader sequence number from response header
	var remoteReaderSeq uint64
	if resp != nil && resp.Header != nil {
		seqStr := resp.Header.Get(codersdk.HeaderImmortalStreamSequenceNum)
		if seqStr != "" {
			if seq, parseErr := strconv.ParseUint(seqStr, 10, 64); parseErr == nil {
				remoteReaderSeq = seq
			}
		}
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}
	r.logger.Debug(ctx, "immortal reconnect upgraded", slog.F("url", wsURL), slog.F("remote_reader_seq", remoteReaderSeq))

	// Convert to net.Conn for binary transport
	nc := websocket.NetConn(ctx, ws, websocket.MessageBinary)
	r.logger.Debug(ctx, "immortal reconnect returning stream")

	// Return the connection and remote reader sequence for writer replay.
	return nc, remoteReaderSeq, nil
}

// getAgentConn returns the current agent connection under a read lock.
func (r *clientStreamReconnector) getAgentConn() workspacesdk.AgentConn {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.agentConn
}

// refreshAgentConn reacquires a fresh AgentConn and swaps it in atomically.
func (r *clientStreamReconnector) refreshAgentConn(ctx context.Context) error {
	opts := r.dialOpts
	if opts == nil {
		opts = &workspacesdk.DialAgentOptions{Logger: r.logger}
	}
	newConn, err := workspacesdk.New(r.client).DialAgent(ctx, r.agentID, opts)
	if err != nil {
		return err
	}
	var old workspacesdk.AgentConn
	r.mu.Lock()
	old = r.agentConn
	r.agentConn = newConn
	r.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	r.logger.Info(ctx, "refreshed agent connection for immortal stream reconnect", slog.F("agent_id", r.agentID))
	return nil
}

// shouldRefreshOnDialError determines whether we should refresh the AgentConn on dial failure.
func (*clientStreamReconnector) shouldRefreshOnDialError(resp *http.Response, err error) bool {
	// If no HTTP response, it's likely a transport-level failure.
	if resp == nil {
		return true
	}

	// Inspect error message for common transient/unreachable conditions.
	if err == nil {
		return false
	}
	low := strings.ToLower(err.Error())
	if strings.Contains(low, "not reachable") ||
		strings.Contains(low, "context deadline exceeded") ||
		strings.Contains(low, "timeout") ||
		strings.Contains(low, "i/o timeout") ||
		strings.Contains(low, "connection refused") {
		return true
	}

	// Also consider network op errors as refreshable.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	return false
}

func (c *immortalBackedConn) Read(p []byte) (int, error)  { return c.pipe.Read(p) }
func (c *immortalBackedConn) Write(p []byte) (int, error) { return c.pipe.Write(p) }
func (c *immortalBackedConn) Close() error {
	c.cancel()
	return c.pipe.Close()
}

// The following implement net.Conn; they are best-effort/no-op where not applicable.
func (*immortalBackedConn) LocalAddr() net.Addr                { return nil }
func (*immortalBackedConn) RemoteAddr() net.Addr               { return nil }
func (*immortalBackedConn) SetDeadline(t time.Time) error      { _ = t; return nil }
func (*immortalBackedConn) SetReadDeadline(t time.Time) error  { _ = t; return nil }
func (*immortalBackedConn) SetWriteDeadline(t time.Time) error { _ = t; return nil }

// startSupervisor keeps attempting reconnection while disconnected.
func (c *immortalBackedConn) startSupervisor() {
	go func() {
		for {
			select {
			case <-c.ctx.Done():
				return
			default:
			}

			// Attempt reconnect if not connected
			if !c.pipe.Connected() {
				if err := c.pipe.ForceReconnect(); err != nil {
					c.logger.Error(context.Background(), "backedpipe reconnect attempt failed", slog.Error(err), slog.F("interval", (3*time.Second).String()))
				}
			}

			// Fixed retry cadence
			select {
			case <-time.After(3 * time.Second):
			case <-c.ctx.Done():
				return
			}
		}
	}()
}
