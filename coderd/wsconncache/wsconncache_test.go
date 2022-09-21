package wsconncache_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/wsconncache"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/tailnet"
	"github.com/coder/coder/tailnet/tailnettest"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestCache(t *testing.T) {
	t.Parallel()
	t.Run("Same", func(t *testing.T) {
		t.Parallel()
		cache := wsconncache.New(func(r *http.Request, id uuid.UUID) (*codersdk.AgentConn, error) {
			return setupAgent(t, codersdk.WorkspaceAgentMetadata{}, 0), nil
		}, 0)
		defer func() {
			_ = cache.Close()
		}()
		conn1, _, err := cache.Acquire(httptest.NewRequest(http.MethodGet, "/", nil), uuid.Nil)
		require.NoError(t, err)
		conn2, _, err := cache.Acquire(httptest.NewRequest(http.MethodGet, "/", nil), uuid.Nil)
		require.NoError(t, err)
		require.True(t, conn1 == conn2)
	})
	t.Run("Expire", func(t *testing.T) {
		t.Parallel()
		called := atomic.NewInt32(0)
		cache := wsconncache.New(func(r *http.Request, id uuid.UUID) (*codersdk.AgentConn, error) {
			called.Add(1)
			return setupAgent(t, codersdk.WorkspaceAgentMetadata{}, 0), nil
		}, time.Microsecond)
		defer func() {
			_ = cache.Close()
		}()
		conn, release, err := cache.Acquire(httptest.NewRequest(http.MethodGet, "/", nil), uuid.Nil)
		require.NoError(t, err)
		release()
		<-conn.Closed()
		conn, release, err = cache.Acquire(httptest.NewRequest(http.MethodGet, "/", nil), uuid.Nil)
		require.NoError(t, err)
		release()
		<-conn.Closed()
		require.Equal(t, int32(2), called.Load())
	})
	t.Run("NoExpireWhenLocked", func(t *testing.T) {
		t.Parallel()
		cache := wsconncache.New(func(r *http.Request, id uuid.UUID) (*codersdk.AgentConn, error) {
			return setupAgent(t, codersdk.WorkspaceAgentMetadata{}, 0), nil
		}, time.Microsecond)
		defer func() {
			_ = cache.Close()
		}()
		conn, release, err := cache.Acquire(httptest.NewRequest(http.MethodGet, "/", nil), uuid.Nil)
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
		release()
		<-conn.Closed()
	})
	t.Run("HTTPTransport", func(t *testing.T) {
		t.Parallel()
		random, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer func() {
			_ = random.Close()
		}()
		tcpAddr, valid := random.Addr().(*net.TCPAddr)
		require.True(t, valid)

		server := &http.Server{
			ReadHeaderTimeout: time.Minute,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
		}
		defer func() {
			_ = server.Close()
		}()
		go server.Serve(random)

		cache := wsconncache.New(func(r *http.Request, id uuid.UUID) (*codersdk.AgentConn, error) {
			return setupAgent(t, codersdk.WorkspaceAgentMetadata{}, 0), nil
		}, time.Microsecond)
		defer func() {
			_ = cache.Close()
		}()

		var wg sync.WaitGroup
		// Perform many requests in parallel to simulate
		// simultaneous HTTP requests.
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				proxy := httputil.NewSingleHostReverseProxy(&url.URL{
					Scheme: "http",
					Host:   fmt.Sprintf("127.0.0.1:%d", tcpAddr.Port),
					Path:   "/",
				})
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				conn, release, err := cache.Acquire(req, uuid.Nil)
				if !assert.NoError(t, err) {
					return
				}
				defer release()
				proxy.Transport = conn.HTTPTransport()
				res := httptest.NewRecorder()
				proxy.ServeHTTP(res, req)
				resp := res.Result()
				defer resp.Body.Close()
				assert.Equal(t, http.StatusOK, resp.StatusCode)
			}()
		}
		wg.Wait()
	})
}

func setupAgent(t *testing.T, metadata codersdk.WorkspaceAgentMetadata, ptyTimeout time.Duration) *codersdk.AgentConn {
	metadata.DERPMap = tailnettest.RunDERPAndSTUN(t)

	coordinator := tailnet.NewCoordinator()
	agentID := uuid.New()
	closer := agent.New(agent.Options{
		FetchMetadata: func(ctx context.Context) (codersdk.WorkspaceAgentMetadata, error) {
			return metadata, nil
		},
		CoordinatorDialer: func(ctx context.Context) (net.Conn, error) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() {
				_ = serverConn.Close()
				_ = clientConn.Close()
			})
			go coordinator.ServeAgent(serverConn, agentID)
			return clientConn, nil
		},
		Logger:                 slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelInfo),
		ReconnectingPTYTimeout: ptyTimeout,
	})
	t.Cleanup(func() {
		_ = closer.Close()
	})
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		DERPMap:   metadata.DERPMap,
		Logger:    slogtest.Make(t, nil).Named("tailnet").Leveled(slog.LevelDebug),
	})
	require.NoError(t, err)
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		_ = conn.Close()
	})
	go coordinator.ServeClient(serverConn, uuid.New(), agentID)
	sendNode, _ := tailnet.ServeCoordinator(clientConn, func(node []*tailnet.Node) error {
		return conn.UpdateNodes(node)
	})
	conn.SetNodeCallback(sendNode)
	return &codersdk.AgentConn{
		Conn: conn,
	}
}
