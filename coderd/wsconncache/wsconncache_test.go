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
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"
	"storj.io/drpc"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/wsconncache"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	drpcsdk "github.com/coder/coder/v2/codersdk/drpc"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
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
			return setupAgent(t, agentsdk.Manifest{}, 0)
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
		called := int32(0)
		cache := wsconncache.New(func(id uuid.UUID) (*codersdk.WorkspaceAgentConn, error) {
			atomic.AddInt32(&called, 1)
			return setupAgent(t, agentsdk.Manifest{}, 0)
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
		require.Equal(t, int32(2), called)
	})
	t.Run("NoExpireWhenLocked", func(t *testing.T) {
		t.Parallel()
		cache := wsconncache.New(func(id uuid.UUID) (*codersdk.WorkspaceAgentConn, error) {
			return setupAgent(t, agentsdk.Manifest{}, 0)
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
			return setupAgent(t, agentsdk.Manifest{}, 0)
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

func setupAgent(t *testing.T, manifest agentsdk.Manifest, ptyTimeout time.Duration) (*codersdk.WorkspaceAgentConn, error) {
	t.Helper()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	manifest.DERPMap, _ = tailnettest.RunDERPAndSTUN(t)

	coordinator := tailnet.NewCoordinator(logger)
	t.Cleanup(func() {
		_ = coordinator.Close()
	})
	manifest.AgentID = uuid.New()
	aC := newClient(
		t,
		slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		manifest,
		coordinator,
	)
	t.Cleanup(aC.close)
	closer := agent.New(agent.Options{
		Client:                 aC,
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
	// setupAgent is called by wsconncache Dialer, so we can't use require here as it will end the
	// test, which in turn closes the wsconncache, which in turn waits for the Dialer and deadlocks.
	if !assert.NoError(t, err) {
		return nil, err
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})
	clientID := uuid.New()
	testCtx, testCtxCancel := context.WithCancel(context.Background())
	t.Cleanup(testCtxCancel)
	coordination := tailnet.NewInMemoryCoordination(
		testCtx, logger,
		clientID, manifest.AgentID,
		coordinator, conn,
	)
	t.Cleanup(func() {
		_ = coordination.Close()
	})
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
		// setupAgent is called by wsconncache Dialer, so we can't use t.Fatal here as it will end
		// the test, which in turn closes the wsconncache, which in turn waits for the Dialer and
		// deadlocks.
		t.Error("agent not reachable")
		return nil, xerrors.New("agent not reachable")
	}
	return agentConn, nil
}

type client struct {
	t              *testing.T
	agentID        uuid.UUID
	manifest       agentsdk.Manifest
	coordinator    tailnet.Coordinator
	closeOnce      sync.Once
	derpMapUpdates chan *tailcfg.DERPMap
	server         *drpcserver.Server
	fakeAgentAPI   *agenttest.FakeAgentAPI
}

func newClient(t *testing.T, logger slog.Logger, manifest agentsdk.Manifest, coordinator tailnet.Coordinator) *client {
	logger = logger.Named("drpc")
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coordinator)
	mux := drpcmux.New()
	derpMapUpdates := make(chan *tailcfg.DERPMap)
	drpcService := &tailnet.DRPCService{
		CoordPtr:               &coordPtr,
		Logger:                 logger,
		DerpMapUpdateFrequency: time.Microsecond,
		DerpMapFn:              func() *tailcfg.DERPMap { return <-derpMapUpdates },
	}
	err := proto.DRPCRegisterTailnet(mux, drpcService)
	require.NoError(t, err)
	fakeAAPI := agenttest.NewFakeAgentAPI(t, logger)
	err = agentproto.DRPCRegisterAgent(mux, fakeAAPI)
	require.NoError(t, err)
	server := drpcserver.NewWithOptions(mux, drpcserver.Options{
		Log: func(err error) {
			if xerrors.Is(err, io.EOF) {
				return
			}
			logger.Debug(context.Background(), "drpc server error", slog.Error(err))
		},
	})

	return &client{
		t:              t,
		agentID:        manifest.AgentID,
		manifest:       manifest,
		coordinator:    coordinator,
		derpMapUpdates: derpMapUpdates,
		server:         server,
		fakeAgentAPI:   fakeAAPI,
	}
}

func (c *client) close() {
	c.closeOnce.Do(func() { close(c.derpMapUpdates) })
}

func (c *client) Manifest(_ context.Context) (agentsdk.Manifest, error) {
	return c.manifest, nil
}

func (c *client) Listen(_ context.Context) (drpc.Conn, error) {
	conn, lis := drpcsdk.MemTransportPipe()
	c.t.Cleanup(func() {
		_ = conn.Close()
		_ = lis.Close()
	})

	serveCtx, cancel := context.WithCancel(context.Background())
	c.t.Cleanup(cancel)
	auth := tailnet.AgentTunnelAuth{}
	streamID := tailnet.StreamID{
		Name: "wsconncache_test-agent",
		ID:   c.agentID,
		Auth: auth,
	}
	serveCtx = tailnet.WithStreamID(serveCtx, streamID)
	go func() {
		c.server.Serve(serveCtx, lis)
	}()
	return conn, nil
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

func (*client) PostMetadata(_ context.Context, _ agentsdk.PostMetadataRequest) error {
	return nil
}

func (*client) PostStartup(_ context.Context, _ agentsdk.PostStartupRequest) error {
	return nil
}

func (*client) PatchLogs(_ context.Context, _ agentsdk.PatchLogs) error {
	return nil
}
