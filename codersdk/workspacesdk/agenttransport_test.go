package workspacesdk_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
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

func TestAgentAPITransport(t *testing.T) {
	t.Parallel()

	t.Run("IgnoresHost", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		var port atomic.Int32
		tr := workspacesdk.NewAgentAPITransport(serverDialer(okServer(t), &port))
		t.Cleanup(tr.CloseIdleConnections)

		// .invalid never resolves, so success means the host was not used.
		require.NoError(t, roundTrip(ctx, tr, "http://other-agent.invalid:4/"))
		require.EqualValues(t, workspacesdk.AgentHTTPAPIServerPort, port.Load())
	})

	t.Run("RejectsOtherPorts", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		var port atomic.Int32
		tr := workspacesdk.NewAgentAPITransport(serverDialer(okServer(t), &port))
		t.Cleanup(tr.CloseIdleConnections)

		require.Error(t, roundTrip(ctx, tr, "http://agent.invalid:80/"))
		require.Zero(t, port.Load(), "rejected port must not be dialed")
	})

	t.Run("ReusesConnection", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		var port atomic.Int32
		tr := workspacesdk.NewAgentAPITransport(serverDialer(okServer(t), &port))
		t.Cleanup(tr.CloseIdleConnections)

		require.NoError(t, roundTrip(ctx, tr, "http://agent.invalid:4/"))
		var reused bool
		traceCtx := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
			GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused },
		})
		require.NoError(t, roundTrip(traceCtx, tr, "http://agent.invalid:4/"))
		require.True(t, reused)
	})

	t.Run("CanceledRequestCancelsDial", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		dialStarted := make(chan struct{})
		dialEnded := make(chan struct{})
		tr := workspacesdk.NewAgentAPITransport(func(ctx context.Context, _ uint16) (net.Conn, error) {
			close(dialStarted)
			<-ctx.Done() // An unreachable agent.
			close(dialEnded)
			return nil, ctx.Err()
		})
		t.Cleanup(tr.CloseIdleConnections)

		reqCtx, cancel := context.WithCancel(ctx)
		go func() {
			<-dialStarted
			cancel()
		}()
		require.Error(t, roundTrip(reqCtx, tr, "http://agent.invalid:4/"))
		testutil.TryReceive(ctx, t, dialEnded)
	})
}

func TestAgentAppTransport_Ports(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	var port atomic.Int32
	tr := workspacesdk.NewAgentAppTransport(serverDialer(okServer(t), &port))
	t.Cleanup(tr.CloseIdleConnections)

	require.Error(t, roundTrip(ctx, tr, "http://agent.invalid:8/"))
	require.Zero(t, port.Load(), "reserved port must not be dialed")
	require.NoError(t, roundTrip(ctx, tr, "http://agent.invalid:9/"))
	require.EqualValues(t, workspacesdk.AgentMinimumListeningPort, port.Load())
}
