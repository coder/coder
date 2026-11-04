package coderd

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkloadHostConnectionReuse(t *testing.T) {
	t.Parallel()
	payload := strings.Repeat("x", 32<<10)
	f := newWorkloadTransportTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer dedicated-supervisor-token", r.Header.Get("Authorization"))
		assert.NoError(t, json.NewEncoder(w).Encode(payload))
	}))
	for range 100 {
		var result string
		_, err := f.api.workloadHostRequest(t.Context(), f.host, http.MethodGet, "/info", nil, &result)
		require.NoError(t, err)
		require.Equal(t, payload, result)
		// Even responses that callers ignore must be consumed for reuse.
		_, err = f.api.workloadHostRequest(t.Context(), f.host, http.MethodPost, "/drain", nil, nil)
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, f.acquires.Load())
	require.Zero(t, f.releases.Load(), "an idle socket must retain its provider lease")
	f.cancel()
	testutil.TryReceive(testutil.Context(t, testutil.WaitLong), t, f.released)
	require.Equal(t, f.acquires.Load(), f.releases.Load())
}

func TestWorkloadHostConnectionPoolSeparatesAgents(t *testing.T) {
	t.Parallel()
	first := newWorkloadTransportTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer dedicated-supervisor-token", r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, `"first"`)
	}))
	second := newWorkloadTransportTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer second-supervisor-token", r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, `"second"`)
	}))
	second.host.token = "second-supervisor-token"
	firstProvider, secondProvider := first.api.agentProvider, second.api.agentProvider
	first.api.agentProvider = fakeAgentProvider{agentConn: func(ctx context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		if id == second.host.agentID {
			return secondProvider.AgentConn(ctx, id)
		}
		return firstProvider.AgentConn(ctx, id)
	}}
	for range 5 {
		for _, tc := range []struct {
			host *workloadHost
			want string
		}{{first.host, "first"}, {second.host, "second"}} {
			var result string
			_, err := first.api.workloadHostRequest(t.Context(), tc.host, http.MethodGet, "/info", nil, &result)
			require.NoError(t, err)
			require.Equal(t, tc.want, result)
		}
	}
	require.EqualValues(t, 1, first.acquires.Load())
	require.EqualValues(t, 1, second.acquires.Load())
	first.cancel()
	ctx := testutil.Context(t, testutil.WaitLong)
	testutil.TryReceive(ctx, t, first.released)
	testutil.TryReceive(ctx, t, second.released)
}

func TestWorkloadHostPOSTDoesNotReplay(t *testing.T) {
	t.Parallel()
	for _, body := range []any{nil, map[string]string{"command": "run once"}} {
		name := "Drain"
		if body != nil {
			name = "Dispatch"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var posts atomic.Int64
			f := newWorkloadTransportTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					_, _ = io.WriteString(w, "{}")
					return
				}
				posts.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				conn, _, err := w.(http.Hijacker).Hijack()
				if assert.NoError(t, err) {
					_ = conn.Close()
				}
			}))
			_, err := f.api.workloadHostRequest(t.Context(), f.host, http.MethodGet, "/info", nil, nil)
			require.NoError(t, err)
			_, err = f.api.workloadHostRequest(t.Context(), f.host, http.MethodPost, "/workloads", body, nil)
			require.EqualError(t, err, "workload supervisor request failed")
			require.EqualValues(t, 1, posts.Load())
			require.EqualValues(t, 1, f.acquires.Load(), "the POST must use the existing connection without retrying")
		})
	}
}

func TestWorkloadHostPOSTWriteFailureDoesNotRetry(t *testing.T) {
	t.Parallel()
	for _, body := range []any{nil, map[string]string{"command": "run once"}} {
		name := "Drain"
		if body != nil {
			name = "Dispatch"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newWorkloadTransportTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, "{}")
			}))
			var fail atomic.Bool
			f.wrap = func(conn net.Conn) net.Conn { return &workloadWriteFailureConn{Conn: conn, fail: &fail} }
			_, err := f.api.workloadHostRequest(t.Context(), f.host, http.MethodGet, "/info", nil, nil)
			require.NoError(t, err)
			fail.Store(true)
			_, err = f.api.workloadHostRequest(t.Context(), f.host, http.MethodPost, "/workloads", body, nil)
			require.EqualError(t, err, "workload supervisor request failed")
			require.EqualValues(t, 1, f.acquires.Load(), "even a zero-byte POST write failure must not trigger a retry")
		})
	}
}

func TestWorkloadHostConcurrentRequests(t *testing.T) {
	t.Parallel()
	var posts atomic.Int64
	f := newWorkloadTransportTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		_, _ = io.WriteString(w, "{}")
	}))
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_, err := f.api.workloadHostRequest(t.Context(), f.host, http.MethodPost, "/workloads", struct{}{}, nil)
			assert.NoError(t, err)
		})
	}
	wg.Wait()
	require.EqualValues(t, 20, posts.Load())
	f.api.workloadTransport().CloseIdleConnections()
	ctx := testutil.Context(t, testutil.WaitLong)
	for range f.acquires.Load() {
		testutil.TryReceive(ctx, t, f.released)
	}
	require.Equal(t, f.acquires.Load(), f.releases.Load())
}

func TestWorkloadHostCanceledRequestDoesNotReplay(t *testing.T) {
	t.Parallel()
	started, proceed := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(proceed) })
	defer unblock()
	var posts atomic.Int64
	f := newWorkloadTransportTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		close(started)
		<-proceed
	}))
	ctx := testutil.Context(t, testutil.WaitLong)
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := f.api.workloadHostRequest(requestCtx, f.host, http.MethodPost, "/workloads", struct{}{}, nil)
		done <- err
	}()
	testutil.TryReceive(ctx, t, started)
	cancel()
	require.EqualError(t, testutil.TryReceive(ctx, t, done), "workload supervisor request failed")
	testutil.TryReceive(ctx, t, f.released)
	require.EqualValues(t, 1, posts.Load())
	require.EqualValues(t, 1, f.acquires.Load())
	require.EqualValues(t, 1, f.releases.Load())
}

func TestWorkloadHostResponseLimit(t *testing.T) {
	t.Parallel()
	f := newWorkloadTransportTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", 8<<20))
	}))
	_, err := f.api.workloadHostRequest(t.Context(), f.host, http.MethodGet, "/info", nil, nil)
	require.EqualError(t, err, "workload supervisor request failed")
	testutil.TryReceive(testutil.Context(t, testutil.WaitLong), t, f.released)
	require.EqualValues(t, 1, f.releases.Load())
}

func TestWorkloadHostDialFailureReleasesProvider(t *testing.T) {
	t.Parallel()
	f := newWorkloadTransportTest(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	f.dial = func(context.Context) (net.Conn, error) {
		return nil, xerrors.New("private transport failure")
	}
	_, err := f.api.workloadHostRequest(t.Context(), f.host, http.MethodPost, "/workloads", struct{}{}, nil)
	require.EqualError(t, err, "workload supervisor request failed")
	require.EqualValues(t, 1, f.acquires.Load())
	require.EqualValues(t, 1, f.releases.Load())
}

func TestWorkloadHostShutdownCancelsDial(t *testing.T) {
	t.Parallel()
	f := newWorkloadTransportTest(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	dialing := make(chan struct{})
	f.dial = func(ctx context.Context) (net.Conn, error) {
		close(dialing)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx := testutil.Context(t, testutil.WaitLong)
	done := make(chan error, 1)
	go func() {
		_, err := f.api.workloadHostRequest(ctx, f.host, http.MethodPost, "/workloads", struct{}{}, nil)
		done <- err
	}()
	testutil.TryReceive(ctx, t, dialing)
	f.cancel()
	require.EqualError(t, testutil.TryReceive(ctx, t, done), "workload supervisor request failed")
	testutil.TryReceive(ctx, t, f.released)
	require.EqualValues(t, 1, f.releases.Load())
}

type workloadWriteFailureConn struct {
	net.Conn
	fail *atomic.Bool
}

func (c *workloadWriteFailureConn) Write(p []byte) (int, error) {
	if c.fail.Load() {
		return 0, xerrors.New("private write failure")
	}
	return c.Conn.Write(p)
}

type workloadTransportTest struct {
	api      *API
	host     *workloadHost
	cancel   context.CancelFunc
	acquires atomic.Int64
	releases atomic.Int64
	released chan struct{}
	dial     func(context.Context) (net.Conn, error)
	wrap     func(net.Conn) net.Conn
}

func newWorkloadTransportTest(t *testing.T, handler http.Handler) *workloadTransportTest {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	f := &workloadTransportTest{
		api: &API{ctx: ctx}, host: &workloadHost{agentID: uuid.New(), token: "dedicated-supervisor-token"},
		cancel: cancel, released: make(chan struct{}, 1000),
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Cleanup(func() {
		cancel()
		f.api.workloadTransport().CloseIdleConnections()
	})
	f.dial = func(ctx context.Context) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
		if err == nil && f.wrap != nil {
			conn = f.wrap(conn)
		}
		return conn, err
	}
	agent := agentconnmock.NewMockAgentConn(gomock.NewController(t))
	addr := netip.AddrPortFrom(tailnet.TailscaleServicePrefix.AddrFromUUID(f.host.agentID), workspacesdk.AgentHTTPAPIServerPort).String()
	agent.EXPECT().DialContext(gomock.Any(), "tcp", addr).DoAndReturn(func(ctx context.Context, _, _ string) (net.Conn, error) {
		return f.dial(ctx)
	}).AnyTimes()
	f.api.agentProvider = fakeAgentProvider{agentConn: func(_ context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		assert.Equal(t, f.host.agentID, id)
		f.acquires.Add(1)
		var released atomic.Bool
		return agent, func() {
			assert.False(t, released.Swap(true), "a provider lease must be released only once")
			f.releases.Add(1)
			f.released <- struct{}{}
		}, nil
	}}
	return f
}
