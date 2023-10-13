package wsconncache_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"strings"
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
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/coderd/wsconncache"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestCache(t *testing.T) {
	t.Parallel()
	t.Run("Same", func(t *testing.T) {
		t.Parallel()
		cache := wsconncache.New(func(id uuid.UUID) (*codersdk.WorkspaceAgentConn, error) {
			return setupAgent(t, agentsdk.Manifest{}, 0), nil
		}, 0)
		defer func() {
			_ = cache.Close()
		}()
		conn1, _, err := cache.Acquire(uuid.Nil)
		require.NoError(t, err)
		conn2, _, err := cache.Acquire(uuid.Nil)
		require.NoError(t, err)
		require.True(t, conn1 == conn2)
	})
	t.Run("Expire", func(t *testing.T) {
		t.Parallel()
		called := atomic.NewInt32(0)
		cache := wsconncache.New(func(id uuid.UUID) (*codersdk.WorkspaceAgentConn, error) {
			called.Add(1)
			return setupAgent(t, agentsdk.Manifest{}, 0), nil
		}, time.Microsecond)
		defer func() {
			_ = cache.Close()
		}()
		conn, release, err := cache.Acquire(uuid.Nil)
		require.NoError(t, err)
		release()
		<-conn.Closed()
		conn, release, err = cache.Acquire(uuid.Nil)
		require.NoError(t, err)
		release()
		<-conn.Closed()
		require.Equal(t, int32(2), called.Load())
	})
	t.Run("NoExpireWhenLocked", func(t *testing.T) {
		t.Parallel()
		cache := wsconncache.New(func(id uuid.UUID) (*codersdk.WorkspaceAgentConn, error) {
			return setupAgent(t, agentsdk.Manifest{}, 0), nil
		}, time.Microsecond)
		defer func() {
			_ = cache.Close()
		}()
		conn, release, err := cache.Acquire(uuid.Nil)
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

		cache := wsconncache.New(func(id uuid.UUID) (*codersdk.WorkspaceAgentConn, error) {
			return setupAgent(t, agentsdk.Manifest{}, 0), nil
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
				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
				defer cancel()
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req = req.WithContext(ctx)
				conn, release, err := cache.Acquire(uuid.Nil)
				if !assert.NoError(t, err) {
					return
				}
				defer release()
				if !conn.AwaitReachable(ctx) {
					t.Error("agent not reachable")
					return
				}

				transport := conn.HTTPTransport()
				defer transport.CloseIdleConnections()
				proxy.Transport = transport
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

func setupAgent(t *testing.T, manifest agentsdk.Manifest, ptyTimeout time.Duration) *codersdk.WorkspaceAgentConn {
	t.Helper()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	manifest.DERPMap, _ = tailnettest.RunDERPAndSTUN(t)

	coordinator := tailnet.NewCoordinator(logger)
	t.Cleanup(func() {
		_ = coordinator.Close()
	})
	manifest.AgentID = uuid.New()
	closer := agent.New(agent.Options{
		Client: &client{
			t:           t,
			agentID:     manifest.AgentID,
			manifest:    manifest,
			coordinator: coordinator,
		},
		Logger:                 logger.Named("agent"),
		ReconnectingPTYTimeout: ptyTimeout,
		Addresses:              []netip.Prefix{netip.PrefixFrom(codersdk.WorkspaceAgentIP, 128)},
	})
	t.Cleanup(func() {
		_ = closer.Close()
	})
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses:           []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		DERPMap:             manifest.DERPMap,
		DERPForceWebSockets: manifest.DERPForceWebSockets,
		Logger:              slogtest.Make(t, nil).Named("tailnet").Leveled(slog.LevelDebug),
	})
	require.NoError(t, err)
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		_ = conn.Close()
	})
	go coordinator.ServeClient(serverConn, uuid.New(), manifest.AgentID)
	sendNode, _ := tailnet.ServeCoordinator(clientConn, func(nodes []*tailnet.Node) error {
		return conn.UpdateNodes(nodes, false)
	})
	conn.SetNodeCallback(sendNode)
	agentConn := codersdk.NewWorkspaceAgentConn(conn, codersdk.WorkspaceAgentConnOptions{
		AgentID: manifest.AgentID,
		AgentIP: codersdk.WorkspaceAgentIP,
	})
	t.Cleanup(func() {
		_ = agentConn.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	if !agentConn.AwaitReachable(ctx) {
		t.Fatal("agent not reachable")
	}
	return agentConn
}

type client struct {
	t           *testing.T
	agentID     uuid.UUID
	manifest    agentsdk.Manifest
	coordinator tailnet.Coordinator
}

func (c *client) Manifest(_ context.Context) (agentsdk.Manifest, error) {
	return c.manifest, nil
}

type closer struct {
	closeFunc func() error
}

func (c *closer) Close() error {
	return c.closeFunc()
}

func (*client) DERPMapUpdates(_ context.Context) (<-chan agentsdk.DERPMapUpdate, io.Closer, error) {
	closed := make(chan struct{})
	return make(<-chan agentsdk.DERPMapUpdate), &closer{
		closeFunc: func() error {
			close(closed)
			return nil
		},
	}, nil
}

func (c *client) Listen(_ context.Context) (net.Conn, error) {
	clientConn, serverConn := net.Pipe()
	closed := make(chan struct{})
	c.t.Cleanup(func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
		<-closed
	})
	go func() {
		_ = c.coordinator.ServeAgent(serverConn, c.agentID, "")
		close(closed)
	}()
	return clientConn, nil
}

func (*client) ReportStats(_ context.Context, _ slog.Logger, _ <-chan *agentsdk.Stats, _ func(time.Duration)) (io.Closer, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (*client) PostLifecycle(_ context.Context, _ agentsdk.PostLifecycleRequest) error {
	return nil
}

func (*client) PostAppHealth(_ context.Context, _ agentsdk.PostAppHealthsRequest) error {
	return nil
}

func (*client) PostMetadata(_ context.Context, _ string, _ agentsdk.PostMetadataRequestDeprecated) error {
	return nil
}

func (*client) PostStartup(_ context.Context, _ agentsdk.PostStartupRequest) error {
	return nil
}

func (*client) PatchLogs(_ context.Context, _ agentsdk.PatchLogs) error {
	return nil
}

func (*client) GetServiceBanner(_ context.Context) (codersdk.ServiceBannerConfig, error) {
	return codersdk.ServiceBannerConfig{}, nil
}
