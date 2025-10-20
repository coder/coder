package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
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
	agentConn workspacesdk.AgentConn
	client    *codersdk.Client
	agentID   uuid.UUID
	streamID  uuid.UUID
	logger    slog.Logger

	// precomputed strategy
	wsURL      string
	httpClient *http.Client

	// onPermanentFailure, if set, is invoked asynchronously when a non-recoverable
	// condition is detected during reconnect (e.g., HTTP 404 from upgrade).
	onPermanentFailure func()
}

// newClientStreamReconnector decides once whether to use Coder Connect or agentConn
// and constructs the static WebSocket URL and HTTP client accordingly.
func newClientStreamReconnector(ctx context.Context, agentConn workspacesdk.AgentConn, client *codersdk.Client, agentID uuid.UUID, streamID uuid.UUID, logger slog.Logger, coderConnectHost string) *clientStreamReconnector {
	wsClient := workspacesdk.New(client)

	// Defaults: use tailnet via agentConn
	apiAddr := fmt.Sprintf("127.0.0.1:%d", workspacesdk.AgentHTTPAPIServerPort)
	wsURL := fmt.Sprintf("ws://%s/api/v0/immortal-stream/%s", apiAddr, streamID)
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
				logger.Info(context.Background(), "immortal: dialing network connection", slog.F("strategy", "agent_conn"), slog.F("network", network), slog.F("addr", addr))
				conn, err := agentConn.DialContext(dialCtx, network, addr)
				if err != nil {
					logger.Error(context.Background(), "immortal: dial attempt failed", slog.F("strategy", "agent_conn"), slog.Error(err))
					return nil, err
				}
				logger.Info(context.Background(), "immortal: dial connected", slog.F("strategy", "agent_conn"))
				return conn, nil
			},
		},
	}

	// Fetch connection info first, then check Coder Connect using the correct suffix
	if connInfo, err := wsClient.AgentConnectionInfoGeneric(ctx); err == nil {
		logger.Info(ctx, "immortal: fetched agent connection info", slog.F("hostname_suffix", connInfo.HostnameSuffix))
		if ok, err := wsClient.IsCoderConnectRunning(ctx, workspacesdk.CoderConnectQueryOptions{HostnameSuffix: connInfo.HostnameSuffix}); err == nil && ok {
			logger.Info(ctx, "immortal: coder connect is running", slog.F("hostname_suffix", connInfo.HostnameSuffix))
			wsURL = fmt.Sprintf("ws://%s:%d/api/v0/immortal-stream/%s", coderConnectHost, workspacesdk.AgentHTTPAPIServerPort, streamID)

			// Use the shared Coder Connect dialer (overridable in tests) instead of a raw net.Dialer
			dialer := testOrDefaultDialer(ctx)
			httpClient = &http.Client{
				Transport: &http.Transport{
					DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
						logger.Info(context.Background(), "immortal: dialing network connection", slog.F("strategy", "coder_connect"), slog.F("network", network), slog.F("addr", addr))
						conn, err := dialer.DialContext(dialCtx, network, addr)
						if err != nil {
							logger.Error(context.Background(), "immortal: dial attempt failed", slog.F("strategy", "coder_connect"), slog.Error(err))
							return nil, err
						}
						logger.Info(context.Background(), "immortal: dial connected", slog.F("strategy", "coder_connect"))
						return conn, nil
					},
				},
			}
			logger.Info(ctx, "immortal reconnector strategy selected", slog.F("strategy", "coder_connect"), slog.F("url", wsURL))
		} else {
			if err != nil {
				logger.Info(ctx, "immortal: coder connect check errored", slog.F("hostname_suffix", connInfo.HostnameSuffix), slog.Error(err))
			} else {
				logger.Info(ctx, "immortal: coder connect not running", slog.F("hostname_suffix", connInfo.HostnameSuffix))
			}
		}
	} else {
		logger.Info(ctx, "immortal: failed to fetch agent connection info", slog.Error(err))
		logger.Info(ctx, "immortal reconnector strategy selected", slog.F("strategy", "agent_conn"), slog.F("url", wsURL))
	}

	return &clientStreamReconnector{
		agentConn:  agentConn,
		client:     client,
		agentID:    agentID,
		streamID:   streamID,
		logger:     logger,
		wsURL:      wsURL,
		httpClient: httpClient,
	}
}

func (r *clientStreamReconnector) Reconnect(ctx context.Context, readerSeqNum uint64) (io.ReadWriteCloser, uint64, error) {
	// Prepare dial options using the precomputed HTTP client.
	dialOptions := &websocket.DialOptions{
		HTTPClient: r.httpClient,
		HTTPHeader: http.Header{
			codersdk.HeaderImmortalStreamSequenceNum: []string{strconv.FormatUint(readerSeqNum, 10)},
		},
		// Negotiate the immortal stream subprotocol with the agent.
		Subprotocols:    []string{codersdk.HeaderUpgradeImmortalStream},
		CompressionMode: websocket.CompressionDisabled,
	}

	// Per-attempt timeout: keep reconnect attempts snappy
	dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	r.logger.Info(ctx, "immortal: attempting reconnect", slog.F("url", r.wsURL), slog.F("reader_seq", readerSeqNum))
	ws, resp, err := websocket.Dial(dialCtx, r.wsURL, dialOptions)
	if err != nil {
		var status string
		var statusCode int
		var hdr http.Header
		var bodyStr string
		if resp != nil {
			status = resp.Status
			statusCode = resp.StatusCode
			hdr = resp.Header.Clone()
			if resp.Body != nil {
				b, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				bodyStr = string(b)
			}
		}
		// If the server returned 404 on upgrade, the immortal stream no longer exists.
		// Tear down the pipe to stop further reconnect attempts.
		if statusCode == http.StatusNotFound {
			r.logger.Info(ctx, "immortal: websocket upgrade returned 404, closing backed pipe", slog.F("url", r.wsURL))
			if r.onPermanentFailure != nil {
				go r.onPermanentFailure()
			}
		}
		r.logger.Error(ctx, "immortal reconnect dial failed", slog.Error(err), slog.F("url", r.wsURL), slog.F("status", status), slog.F("headers", hdr), slog.F("body", bodyStr))
		return nil, 0, xerrors.Errorf("failed to WebSocket dial: %w", err)
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
	r.logger.Info(ctx, "immortal: reconnect established", slog.F("url", r.wsURL), slog.F("remote_reader_seq", remoteReaderSeq))

	// Convert to net.Conn for binary transport
	nc := websocket.NetConn(ctx, ws, websocket.MessageBinary)
	r.logger.Debug(ctx, "immortal reconnect returning stream")

	// Return the connection and remote reader sequence for writer replay.
	return nc, remoteReaderSeq, nil
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
				c.logger.Info(context.Background(), "immortal: supervisor forcing reconnect")
				if err := c.pipe.ForceReconnect(); err != nil {
					// If the pipe is closed, stop supervising.
					if errors.Is(err, io.EOF) {
						c.logger.Info(context.Background(), "immortal: pipe closed, stopping supervisor")
						return
					}
					c.logger.Error(context.Background(), "backedpipe reconnect attempt failed", slog.Error(err))
				}
			}

			select {
			case <-time.After(5 * time.Second):
			case <-c.ctx.Done():
				return
			}
		}
	}()
}
