package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/websocket"
)

// immortalReconnectingConn is a net.Conn that talks to an agent Immortal Stream
// endpoint and transparently reconnects on failures. It preserves read
// sequence state via the X-Coder-Immortal-Stream-Sequence-Num header so the
// server can replay any missed bytes to the client. Writes will block across
// reconnects and resume once a new connection is established.
//
// Note: Without an explicit server-to-client acknowledgement of how many bytes
// of client->server data were consumed, we avoid attempting to replay writes
// from the client. Instead, Write blocks until a new connection is established
// and then continues writing new data. This preserves the SSH session transport
// and prevents premature termination.
type immortalReconnectingConn struct {
    ctx    context.Context
    cancel context.CancelFunc

	agentConn workspacesdk.AgentConn
	streamID  uuid.UUID
	logger    slog.Logger

	mu       sync.Mutex
	ws       *websocket.Conn
	nc       net.Conn
	closed   bool
	readerSN uint64 // total bytes read by this client

    // cancel the per-connection keepalive loop
    keepaliveCancel context.CancelFunc

    // Deduplicate concurrent reconnect attempts
    sf singleflight.Group

    // start the background reconnect supervisor only once
    bgOnce sync.Once

    // Optional: called when the server indicates the stream ID is invalid
    // and a new stream should be created. Returns a replacement stream ID.
    refreshStreamID func(context.Context) (uuid.UUID, error)
}

// newImmortalReconnectingConn constructs a reconnecting connection and dials
// the initial websocket. Subsequent reads/writes will reconnect on demand.
func newImmortalReconnectingConn(parent context.Context, agentConn workspacesdk.AgentConn, streamID uuid.UUID, logger slog.Logger, refresh func(context.Context) (uuid.UUID, error)) (net.Conn, error) {
	ctx, cancel := context.WithCancel(parent)

	// Add connection ID for better logging
	connID := uuid.New()
	logger = logger.With(slog.F("conn_id", connID), slog.F("stream_id", streamID))

    c := &immortalReconnectingConn{
        ctx:       ctx,
        cancel:    cancel,
        agentConn: agentConn,
        streamID:  streamID,
        logger:    logger,
        refreshStreamID: refresh,
    }

	c.logger.Debug(context.Background(), "creating new immortal reconnecting connection")

    if err := c.ensureConnected(); err != nil {
        cancel()
        return nil, xerrors.Errorf("initial connection failed: %w", err)
    }

    c.logger.Debug(context.Background(), "immortal reconnecting connection created successfully")
    // Ensure we always have an out-of-band retry loop so that reconnects
    // continue even when no reader/writer is active.
    c.startReconnectSupervisor()
    return c, nil
}

func (c *immortalReconnectingConn) Read(p []byte) (int, error) {
	for {
		c.mu.Lock()
		nc := c.nc
		closed := c.closed
		c.mu.Unlock()
		if closed {
			c.logger.Debug(context.Background(), "read called on closed connection")
			return 0, net.ErrClosed
		}

		if nc == nil {
			c.logger.Debug(context.Background(), "read called on nil connection, attempting reconnect")
			if err := c.reconnect(); err != nil {
				if c.ctx.Err() != nil {
					c.logger.Debug(context.Background(), "read reconnect failed due to context cancellation", slog.Error(err))
					return 0, c.ctx.Err()
				}
				c.logger.Error(context.Background(), "read reconnect failed, will retry", slog.Error(err))
				// Brief backoff to avoid hot loop
				select {
				case <-time.After(200 * time.Millisecond):
				case <-c.ctx.Done():
					return 0, c.ctx.Err()
				}
				continue
			}
			continue
		}

		n, err := nc.Read(p)
		if n > 0 {
			c.mu.Lock()
			c.readerSN += uint64(n)
			c.mu.Unlock()
			c.logger.Debug(context.Background(), "read successful", slog.F("bytes", n), slog.F("total_read", c.readerSN))
			return n, nil
		}
		if err == nil {
			// zero bytes without error, try again
			continue
		}

		// Read error: trigger reconnect loop and retry
		c.logger.Debug(context.Background(), "immortal read error, reconnecting", slog.Error(err))
		_ = c.reconnect()
		// Loop to retry read on new connection
	}
}

func (c *immortalReconnectingConn) Write(p []byte) (int, error) {
	writtenTotal := 0
	for writtenTotal < len(p) {
		c.mu.Lock()
		nc := c.nc
		closed := c.closed
		c.mu.Unlock()
		if closed {
			c.logger.Debug(context.Background(), "write called on closed connection")
			return writtenTotal, net.ErrClosed
		}
		if nc == nil {
			c.logger.Debug(context.Background(), "write called on nil connection, attempting reconnect")
			if err := c.reconnect(); err != nil {
				if c.ctx.Err() != nil {
					c.logger.Debug(context.Background(), "write reconnect failed due to context cancellation", slog.Error(err))
					return writtenTotal, c.ctx.Err()
				}
				c.logger.Error(context.Background(), "write reconnect failed, will retry", slog.Error(err))
				// Backoff before reattempting
				select {
				case <-time.After(200 * time.Millisecond):
				case <-c.ctx.Done():
					return writtenTotal, c.ctx.Err()
				}
				continue
			}
			continue
		}

		n, err := nc.Write(p[writtenTotal:])
		if n > 0 {
			writtenTotal += n
			c.logger.Debug(context.Background(), "write partial success", slog.F("bytes", n), slog.F("total_written", writtenTotal), slog.F("remaining", len(p)-writtenTotal))
		}
		if err == nil {
			continue
		}
		// Write error: reconnect and retry remaining bytes
		c.logger.Debug(context.Background(), "immortal write error, reconnecting", slog.Error(err))
		if rerr := c.reconnect(); rerr != nil {
			if c.ctx.Err() != nil {
				return writtenTotal, c.ctx.Err()
			}
			c.logger.Error(context.Background(), "write reconnect failed, will retry", slog.Error(rerr))
			// Brief backoff then try again
			select {
			case <-time.After(200 * time.Millisecond):
			case <-c.ctx.Done():
				return writtenTotal, c.ctx.Err()
			}
		}
	}
	c.logger.Debug(context.Background(), "write completed successfully", slog.F("total_bytes", writtenTotal))
	return writtenTotal, nil
}

func (c *immortalReconnectingConn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	ws := c.ws
	nc := c.nc
	kaCancel := c.keepaliveCancel
	c.ws = nil
	c.nc = nil
	c.keepaliveCancel = nil
	c.mu.Unlock()

	c.logger.Debug(context.Background(), "closing immortal reconnecting connection")

	c.cancel()
	if kaCancel != nil {
		kaCancel()
	}

	var firstErr error
	if nc != nil {
		if err := nc.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if ws != nil {
		if err := ws.Close(websocket.StatusNormalClosure, ""); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if firstErr != nil {
		c.logger.Error(context.Background(), "error during connection close", slog.Error(firstErr))
	} else {
		c.logger.Debug(context.Background(), "immortal reconnecting connection closed successfully")
	}

	return firstErr
}

func (c *immortalReconnectingConn) LocalAddr() net.Addr {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nc != nil {
		return c.nc.LocalAddr()
	}
	// best-effort zero addr
	return nil
}

func (c *immortalReconnectingConn) RemoteAddr() net.Addr {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nc != nil {
		return c.nc.RemoteAddr()
	}
	return nil
}

func (c *immortalReconnectingConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nc != nil {
		return c.nc.SetDeadline(t)
	}
	return nil
}

func (c *immortalReconnectingConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nc != nil {
		return c.nc.SetReadDeadline(t)
	}
	return nil
}

func (c *immortalReconnectingConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nc != nil {
		return c.nc.SetWriteDeadline(t)
	}
	return nil
}

// ensureConnected dials the websocket if not currently connected.
func (c *immortalReconnectingConn) ensureConnected() error {
	c.mu.Lock()
	already := c.nc != nil
	c.mu.Unlock()
	if already {
		return nil
	}
	_, err, _ := c.sf.Do("reconnect", func() (any, error) {
		return nil, c.connectOnce()
	})
	return err
}

// reconnect forces a reconnect regardless of current state.
func (c *immortalReconnectingConn) reconnect() error {
	c.logger.Debug(context.Background(), "starting reconnection process")

    _, err, _ := c.sf.Do("reconnect", func() (any, error) {
        // Close any existing connection outside of lock to unblock reader/writer
        c.mu.Lock()
        // stop previous keepalive loop if any
        if c.keepaliveCancel != nil {
            c.logger.Debug(context.Background(), "canceling previous keepalive loop")
            c.keepaliveCancel()
            c.keepaliveCancel = nil
        }
        ws := c.ws
        nc := c.nc
        c.ws = nil
        c.nc = nil
        c.mu.Unlock()

		if nc != nil {
			c.logger.Debug(context.Background(), "closing previous net.Conn")
			_ = nc.Close()
		}
		if ws != nil {
			c.logger.Debug(context.Background(), "closing previous websocket")
			_ = ws.Close(websocket.StatusNormalClosure, "reconnect")
		}

        c.logger.Debug(context.Background(), "attempting new connection")
        return nil, c.connectOnce()
    })

	if err != nil {
		c.logger.Error(context.Background(), "reconnection failed", slog.Error(err))
	} else {
		c.logger.Debug(context.Background(), "reconnection completed successfully")
	}

    // Kick the supervisor so it continues retrying if we failed here.
    if err != nil {
        c.startReconnectSupervisor()
    }
    return err
}

func (c *immortalReconnectingConn) connectOnce() error {
	if c.ctx.Err() != nil {
		c.logger.Debug(context.Background(), "connectOnce called with canceled context", slog.Error(c.ctx.Err()))
		return c.ctx.Err()
	}

	// Build the target address for the agent's HTTP API server
	apiServerAddr := fmt.Sprintf("127.0.0.1:%d", workspacesdk.AgentHTTPAPIServerPort)
	wsURL := fmt.Sprintf("ws://%s/api/v0/immortal-stream/%s", apiServerAddr, c.streamID)

	// Include current reader sequence so the server can replay any missed bytes
	c.mu.Lock()
	readerSeq := c.readerSN
	c.mu.Unlock()

	c.logger.Debug(context.Background(), "dialing websocket",
		slog.F("url", wsURL),
		slog.F("reader_seq", readerSeq))

	dialOptions := &websocket.DialOptions{
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
					c.logger.Debug(context.Background(), "dialing network connection", slog.F("network", network), slog.F("addr", addr))
					return c.agentConn.DialContext(dialCtx, network, addr)
				},
			},
		},
		HTTPHeader: http.Header{
			codersdk.HeaderImmortalStreamSequenceNum: []string{strconv.FormatUint(readerSeq, 10)},
		},
		CompressionMode: websocket.CompressionDisabled,
	}

	// Use a per-attempt dial timeout to avoid indefinite hangs on half-open
	// connections or blackholed networks during reconnect attempts.
	dialCtx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

    ws, resp, err := websocket.Dial(dialCtx, wsURL, dialOptions)
    if err != nil {
        // If we received an HTTP response, inspect status codes that indicate
        // the stream ID is no longer valid and attempt to refresh it.
        if resp != nil {
            status := resp.StatusCode
            _ = resp.Body.Close()
            if (status == http.StatusNotFound || status == http.StatusGone || status == http.StatusBadRequest) && c.refreshStreamID != nil {
                c.logger.Warn(context.Background(), "immortal stream appears invalid; attempting to refresh stream id", slog.F("status", status))
                // Try to obtain a new stream ID and retry once immediately.
                newID, rerr := c.refreshStreamID(c.ctx)
                if rerr == nil {
                    c.mu.Lock()
                    c.streamID = newID
                    c.mu.Unlock()
                    // Rebuild URL with new ID and redial within same attempt context.
                    wsURL = fmt.Sprintf("ws://%s/api/v0/immortal-stream/%s", apiServerAddr, newID)
                    c.logger.Info(context.Background(), "retrying websocket dial with refreshed stream id", slog.F("url", wsURL))
                    ws2, resp2, err2 := websocket.Dial(dialCtx, wsURL, dialOptions)
                    if err2 == nil {
                        ws = ws2
                        // Ensure any intermediate resp2 is closed by NetConn when closed; nothing to do here.
                        goto DIAL_SUCCESS
                    }
                    if resp2 != nil && resp2.Body != nil {
                        _ = resp2.Body.Close()
                    }
                    c.logger.Error(context.Background(), "websocket dial failed after refresh", slog.Error(err2))
                    return xerrors.Errorf("dial immortal stream websocket (after refresh): %w", err2)
                }
                c.logger.Error(context.Background(), "failed to refresh immortal stream id", slog.Error(rerr))
            }
        }
        c.logger.Error(context.Background(), "websocket dial failed", slog.Error(err), slog.F("url", wsURL))
        return xerrors.Errorf("dial immortal stream websocket: %w", err)
    }
DIAL_SUCCESS:
    c.logger.Debug(context.Background(), "websocket dial successful")

	// Convert WebSocket to net.Conn for binary transport
	// Tie lifecycle to our context so reads/writes unblock on shutdown
	nc := websocket.NetConn(c.ctx, ws, websocket.MessageBinary)

	// swap in new connection and start keepalive
	c.mu.Lock()
	// stop previous keepalive loop if any (defensive if connectOnce is called directly)
	if c.keepaliveCancel != nil {
		c.logger.Debug(context.Background(), "canceling existing keepalive loop during connection swap")
		c.keepaliveCancel()
		c.keepaliveCancel = nil
	}
	c.ws = ws
	c.nc = nc
	kaCtx, kaCancel := context.WithCancel(c.ctx)
	c.keepaliveCancel = kaCancel
	c.mu.Unlock()

	// start ping keepalive
	go c.keepaliveLoop(kaCtx, ws)

	c.logger.Debug(context.Background(), "connected to immortal stream", slog.F("stream_id", c.streamID))
	return nil
}

// keepaliveLoop periodically pings the websocket to detect half-open connections.
// On ping failure it triggers a reconnect.
func (c *immortalReconnectingConn) keepaliveLoop(ctx context.Context, ws *websocket.Conn) {
	c.logger.Debug(context.Background(), "starting keepalive loop")

	t := time.NewTicker(1 * time.Second)
	defer t.Stop()

	pingCount := 0
	for {
		select {
		case <-ctx.Done():
			c.logger.Debug(context.Background(), "keepalive loop context canceled", slog.Error(ctx.Err()))
			return
		case <-t.C:
			pingCount++
			pctx, cancel := context.WithTimeout(ctx, time.Second)
			err := ws.Ping(pctx)
			cancel()
			if err != nil {
				c.logger.Debug(context.Background(), "immortal ping failed, reconnecting",
					slog.Error(err),
					slog.F("ping_count", pingCount))
				// Best effort: trigger reconnect to replace dead socket.
				// Don't return - continue monitoring for future failures
				_ = c.reconnect()
				// Continue the loop to monitor the new connection
			} else if pingCount%10 == 0 { // Log every 10th successful ping to avoid spam
				c.logger.Debug(context.Background(), "keepalive ping successful", slog.F("ping_count", pingCount))
			}
		}
	}
}

// startReconnectSupervisor launches a background loop that ensures reconnect attempts
// continue indefinitely while the context is alive, even if there are no active
// Read/Write calls to trigger reconnects.
func (c *immortalReconnectingConn) startReconnectSupervisor() {
    c.bgOnce.Do(func() {
        go func() {
            // Basic capped exponential backoff.
            backoff := 200 * time.Millisecond
            const maxBackoff = 5 * time.Second
            failureCount := 0
            for {
                select {
                case <-c.ctx.Done():
                    return
                default:
                }

                c.mu.Lock()
                hasConn := c.nc != nil
                ws := c.ws
                c.mu.Unlock()

                if hasConn {
                    // Reset backoff when we have a healthy connection.
                    backoff = 200 * time.Millisecond
                    // Actively ping in case the keepalive loop has stopped for any reason.
                    if ws != nil {
                        pctx, cancel := context.WithTimeout(c.ctx, 2*time.Second)
                        if err := ws.Ping(pctx); err != nil {
                            cancel()
                            // Escalate visibility so users see continued retries.
                            c.logger.Error(context.Background(), "supervisor ping failed, forcing reconnect", slog.Error(err))
                            _ = c.reconnect()
                        } else {
                            cancel()
                        }
                    }
                    // Poll sparsely to detect transitions without busy looping.
                    select {
                    case <-time.After(1 * time.Second):
                    case <-c.ctx.Done():
                        return
                    }
                    continue
                }

                // No connection: attempt a reconnect.
                if err := c.ensureConnected(); err != nil {
                    failureCount++
                    // Log as error so it's visible that we're still retrying.
                    c.logger.Error(context.Background(), "background reconnect attempt failed", slog.Error(err), slog.F("attempt", failureCount), slog.F("backoff", backoff.String()))
                    // Backoff and retry until success or context cancel.
                    select {
                    case <-time.After(backoff):
                    case <-c.ctx.Done():
                        return
                    }
                    if backoff < maxBackoff {
                        backoff *= 2
                        if backoff > maxBackoff {
                            backoff = maxBackoff
                        }
                    }
                    continue
                }

                // Success: a keepalive loop is created by connectOnce; loop will
                // continue monitoring in case the connection drops again.
                failureCount = 0
                backoff = 200 * time.Millisecond
            }
        }()
    })
}
