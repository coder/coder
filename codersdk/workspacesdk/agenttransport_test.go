package workspacesdk_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/netip"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

// serverDialer dials srv whatever port it is asked for, and records the
// last port.
func serverDialer(srv *httptest.Server, port *atomic.Int32) workspacesdk.AgentDialer {
	return func(ctx context.Context, p uint16) (net.Conn, error) {
		port.Store(int32(p))
		var d net.Dialer
		return d.DialContext(ctx, "tcp", srv.Listener.Addr().String())
	}
}

func okServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func roundTrip(ctx context.Context, rt http.RoundTripper, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := rt.RoundTrip(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, res.Body)
	return res.Body.Close()
}

func addrURL(addr netip.Addr, port string) string {
	return "http://" + net.JoinHostPort(addr.String(), port) + "/"
}

func TestAgentAPITransport(t *testing.T) {
	t.Parallel()

	apiPort := strconv.Itoa(workspacesdk.AgentHTTPAPIServerPort)

	t.Run("RejectsOtherHosts", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		var port atomic.Int32
		tr := workspacesdk.NewAgentAPITransport(uuid.New(), serverDialer(okServer(t), &port), testutil.Logger(t))
		t.Cleanup(tr.CloseIdleConnections)

		otherAgent := tailnet.TailscaleServicePrefix.AddrFromUUID(uuid.New())
		require.ErrorContains(t, roundTrip(ctx, tr, addrURL(otherAgent, apiPort)), "does not match agent")
		require.ErrorContains(t, roundTrip(ctx, tr, "http://localhost:"+apiPort+"/"), "does not match agent")
		require.Zero(t, port.Load(), "rejected request must not be dialed")
	})

	t.Run("RejectsOtherPorts", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		var port atomic.Int32
		tr := workspacesdk.NewAgentAPITransport(uuid.New(), serverDialer(okServer(t), &port), testutil.Logger(t))
		t.Cleanup(tr.CloseIdleConnections)

		require.ErrorContains(t, roundTrip(ctx, tr, addrURL(tr.Addr(), "80")), "port 80 is not allowed")
		require.Zero(t, port.Load(), "rejected port must not be dialed")
	})

	t.Run("ReusesConnection", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		var port atomic.Int32
		tr := workspacesdk.NewAgentAPITransport(uuid.New(), serverDialer(okServer(t), &port), testutil.Logger(t))
		t.Cleanup(tr.CloseIdleConnections)

		require.NoError(t, roundTrip(ctx, tr, addrURL(tr.Addr(), apiPort)))
		require.EqualValues(t, workspacesdk.AgentHTTPAPIServerPort, port.Load())
		var reused bool
		traceCtx := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
			GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused },
		})
		require.NoError(t, roundTrip(traceCtx, tr, addrURL(tr.Addr(), apiPort)))
		require.True(t, reused)
	})

	t.Run("CanceledRequestCancelsDial", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		dialStarted := make(chan struct{})
		dialEnded := make(chan struct{})
		tr := workspacesdk.NewAgentAPITransport(uuid.New(), func(ctx context.Context, _ uint16) (net.Conn, error) {
			close(dialStarted)
			<-ctx.Done() // An unreachable agent.
			close(dialEnded)
			return nil, ctx.Err()
		}, testutil.Logger(t))
		t.Cleanup(tr.CloseIdleConnections)

		reqCtx, cancel := context.WithCancel(ctx)
		go func() {
			<-dialStarted
			cancel()
		}()
		require.Error(t, roundTrip(reqCtx, tr, addrURL(tr.Addr(), apiPort)))
		testutil.TryReceive(ctx, t, dialEnded)
	})
}

func TestAgentAppTransport(t *testing.T) {
	t.Parallel()

	t.Run("Ports", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		var port atomic.Int32
		tr := workspacesdk.NewAgentAppTransport(uuid.New(), serverDialer(okServer(t), &port), testutil.Logger(t))
		t.Cleanup(tr.CloseIdleConnections)

		require.ErrorContains(t, roundTrip(ctx, tr, addrURL(tr.Addr(), "8")), "port 8 is not allowed")
		require.Zero(t, port.Load(), "reserved port must not be dialed")
		require.NoError(t, roundTrip(ctx, tr, addrURL(tr.Addr(), "9")))
		require.EqualValues(t, workspacesdk.AgentMinimumListeningPort, port.Load())
	})

	t.Run("NoPortUsesSchemeDefault", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		var port atomic.Int32
		tr := workspacesdk.NewAgentAppTransport(uuid.New(), serverDialer(okServer(t), &port), testutil.Logger(t))
		t.Cleanup(tr.CloseIdleConnections)

		// The reverse proxy produces "[addr]:" for an app URL without a port.
		require.NoError(t, roundTrip(ctx, tr, addrURL(tr.Addr(), "")))
		require.EqualValues(t, 80, port.Load())
	})

	t.Run("RejectsOtherHosts", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		var port atomic.Int32
		tr := workspacesdk.NewAgentAppTransport(uuid.New(), serverDialer(okServer(t), &port), testutil.Logger(t))
		t.Cleanup(tr.CloseIdleConnections)

		otherAgent := tailnet.TailscaleServicePrefix.AddrFromUUID(uuid.New())
		require.ErrorContains(t, roundTrip(ctx, tr, addrURL(otherAgent, "8080")), "does not match agent")
		require.ErrorContains(t, roundTrip(ctx, tr, "http://localhost:8080/"), "does not match agent")
		require.Zero(t, port.Load(), "rejected request must not be dialed")
	})
}

// dropIdleListener closes a connection when a request arrives on it after
// a response was written, like an agent that restarted while the
// connection sat idle.
type dropIdleListener struct{ net.Listener }

func (l dropIdleListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &dropIdleConn{Conn: c}, nil
}

type dropIdleConn struct {
	net.Conn
	responded atomic.Bool
}

func (c *dropIdleConn) Write(p []byte) (int, error) {
	c.responded.Store(true)
	return c.Conn.Write(p)
}

func (c *dropIdleConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 && c.responded.Load() {
		_ = c.Close()
		return 0, io.EOF
	}
	return n, err
}

func TestAgentConn_ResendsOnDroppedConnection(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		call   func(context.Context, workspacesdk.AgentConn) error
		resent bool
	}{
		{
			name: "ToolCall",
			call: func(ctx context.Context, conn workspacesdk.AgentConn) error {
				return conn.WriteFile(workspacesdk.WithToolCallID(ctx, uuid.New()), "/tmp/f", strings.NewReader("x"))
			},
			resent: true,
		},
		{
			name: "CancelToolCall",
			call: func(ctx context.Context, conn workspacesdk.AgentConn) error {
				_, err := conn.CancelToolCall(ctx, uuid.New())
				return err
			},
			resent: true,
		},
		{
			name: "NoToolCall",
			call: func(ctx context.Context, conn workspacesdk.AgentConn) error {
				return conn.WriteFile(ctx, "/tmp/f", strings.NewReader("x"))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			var handled atomic.Int32
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				handled.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			srv.Listener = dropIdleListener{srv.Listener}
			srv.Start()
			t.Cleanup(srv.Close)

			agentID := uuid.New()
			var port atomic.Int32
			tr := workspacesdk.NewAgentAPITransport(agentID, serverDialer(srv, &port), testutil.Logger(t))
			t.Cleanup(tr.CloseIdleConnections)
			conn := workspacesdk.NewAgentConn(nil, workspacesdk.AgentConnOptions{
				AgentID:      agentID,
				APITransport: tr,
			})

			require.NoError(t, tc.call(ctx, conn))
			// The second request goes out on the pooled connection, which
			// the server drops.
			err := tc.call(ctx, conn)
			if tc.resent {
				require.NoError(t, err)
				require.EqualValues(t, 2, handled.Load())
			} else {
				require.Error(t, err)
				require.EqualValues(t, 1, handled.Load())
			}
		})
	}
}
