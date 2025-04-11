package coderd_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

func TestServerTailnet_AgentConn_OK(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	// Connect through the ServerTailnet
	agents, serverTailnet := setupServerTailnetAgent(t, 1)
	a := agents[0]

	conn, release, err := serverTailnet.AgentConn(ctx, a.id)
	require.NoError(t, err)
	defer release()

	assert.True(t, conn.AwaitReachable(ctx))
}

func TestServerTailnet_AgentConn_NoSTUN(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	// Connect through the ServerTailnet
	agents, serverTailnet := setupServerTailnetAgent(t, 1, withDERPAndStunOptions(tailnettest.DisableSTUN, tailnettest.DERPIsEmbedded))
	a := agents[0]

	conn, release, err := serverTailnet.AgentConn(ctx, a.id)
	require.NoError(t, err)
	defer release()

	assert.True(t, conn.AwaitReachable(ctx))
}

//nolint:paralleltest // t.Setenv
func TestServerTailnet_ReverseProxy_ProxyEnv(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://169.254.169.254:12345")

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	agents, serverTailnet := setupServerTailnetAgent(t, 1)
	a := agents[0]

	u, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", workspacesdk.AgentHTTPAPIServerPort))
	require.NoError(t, err)

	rp := serverTailnet.ReverseProxy(u, u, a.id, appurl.ApplicationURL{}, "")

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		u.String(),
		nil,
	).WithContext(ctx)

	rp.ServeHTTP(rw, req)
	res := rw.Result()
	defer res.Body.Close()

	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestServerTailnet_ReverseProxy(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agents, serverTailnet := setupServerTailnetAgent(t, 1)
		a := agents[0]

		u, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", workspacesdk.AgentHTTPAPIServerPort))
		require.NoError(t, err)

		rp := serverTailnet.ReverseProxy(u, u, a.id, appurl.ApplicationURL{}, "")

		rw := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodGet,
			u.String(),
			nil,
		).WithContext(ctx)

		rp.ServeHTTP(rw, req)
		res := rw.Result()
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("Metrics", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agents, serverTailnet := setupServerTailnetAgent(t, 1)
		a := agents[0]

		registry := prometheus.NewRegistry()
		require.NoError(t, registry.Register(serverTailnet))

		u, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", workspacesdk.AgentHTTPAPIServerPort))
		require.NoError(t, err)

		rp := serverTailnet.ReverseProxy(u, u, a.id, appurl.ApplicationURL{}, "")

		rw := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodGet,
			u.String(),
			nil,
		).WithContext(ctx)

		rp.ServeHTTP(rw, req)
		res := rw.Result()
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode)
		require.Eventually(t, func() bool {
			metrics, err := registry.Gather()
			assert.NoError(t, err)
			return testutil.PromCounterHasValue(t, metrics, 1, "coder_servertailnet_connections_total", "tcp") &&
				testutil.PromGaugeHasValue(t, metrics, 1, "coder_servertailnet_open_connections", "tcp")
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("HostRewrite", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agents, serverTailnet := setupServerTailnetAgent(t, 1)
		a := agents[0]

		u, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", workspacesdk.AgentHTTPAPIServerPort))
		require.NoError(t, err)

		rp := serverTailnet.ReverseProxy(u, u, a.id, appurl.ApplicationURL{}, "")

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		require.NoError(t, err)

		// Ensure the reverse proxy director rewrites the url host to the agent's IP.
		rp.Director(req)
		assert.Equal(t,
			fmt.Sprintf("[%s]:%d",
				tailnet.TailscaleServicePrefix.AddrFromUUID(a.id).String(),
				workspacesdk.AgentHTTPAPIServerPort),
			req.URL.Host,
		)
	})

	t.Run("CachesConnection", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agents, serverTailnet := setupServerTailnetAgent(t, 1)
		a := agents[0]
		port := ":4444"
		ln, err := a.TailnetConn().Listen("tcp", port)
		require.NoError(t, err)
		wln := &wrappedListener{Listener: ln}

		serverClosed := make(chan struct{})
		go func() {
			defer close(serverClosed)
			//nolint:gosec
			_ = http.Serve(wln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("hello from agent"))
			}))
		}()
		defer func() {
			// wait for server to close
			<-serverClosed
		}()

		defer ln.Close()

		u, err := url.Parse("http://127.0.0.1" + port)
		require.NoError(t, err)

		rp := serverTailnet.ReverseProxy(u, u, a.id, appurl.ApplicationURL{}, "")

		for i := 0; i < 5; i++ {
			rw := httptest.NewRecorder()
			req := httptest.NewRequest(
				http.MethodGet,
				u.String(),
				nil,
			).WithContext(ctx)

			rp.ServeHTTP(rw, req)
			res := rw.Result()

			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)
		}

		assert.Equal(t, 1, wln.getDials())
	})

	t.Run("NotReusedBetweenAgents", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agents, serverTailnet := setupServerTailnetAgent(t, 2)
		port := ":4444"

		for i, ag := range agents {
			i := i
			ln, err := ag.TailnetConn().Listen("tcp", port)
			require.NoError(t, err)
			wln := &wrappedListener{Listener: ln}

			serverClosed := make(chan struct{})
			go func() {
				defer close(serverClosed)
				//nolint:gosec
				_ = http.Serve(wln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(strconv.Itoa(i)))
				}))
			}()
			defer func() { //nolint:revive
				// wait for server to close
				<-serverClosed
			}()

			defer ln.Close() //nolint:revive
		}

		u, err := url.Parse("http://127.0.0.1" + port)
		require.NoError(t, err)

		for i, ag := range agents {
			rp := serverTailnet.ReverseProxy(u, u, ag.id, appurl.ApplicationURL{}, "")

			rw := httptest.NewRecorder()
			req := httptest.NewRequest(
				http.MethodGet,
				u.String(),
				nil,
			).WithContext(ctx)

			rp.ServeHTTP(rw, req)
			res := rw.Result()

			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)
			assert.Equal(t, strconv.Itoa(i), string(body))
		}
	})

	t.Run("HTTPSProxy", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agents, serverTailnet := setupServerTailnetAgent(t, 1)
		a := agents[0]

		const expectedResponseCode = 209
		// Test that we can proxy HTTPS traffic.
		s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(expectedResponseCode)
		}))
		t.Cleanup(s.Close)

		uri, err := url.Parse(s.URL)
		require.NoError(t, err)

		rp := serverTailnet.ReverseProxy(uri, uri, a.id, appurl.ApplicationURL{}, "")

		rw := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodGet,
			uri.String(),
			nil,
		).WithContext(ctx)

		rp.ServeHTTP(rw, req)
		res := rw.Result()
		defer res.Body.Close()

		assert.Equal(t, expectedResponseCode, res.StatusCode)
	})

	t.Run("BlockEndpoints", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agents, serverTailnet := setupServerTailnetAgent(t, 1, withDERPAndStunOptions(tailnettest.DisableSTUN))
		a := agents[0]

		require.True(t, serverTailnet.Conn().GetBlockEndpoints(), "expected BlockEndpoints to be set")

		u, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", workspacesdk.AgentHTTPAPIServerPort))
		require.NoError(t, err)

		rp := serverTailnet.ReverseProxy(u, u, a.id, appurl.ApplicationURL{}, "")

		rw := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodGet,
			u.String(),
			nil,
		).WithContext(ctx)

		rp.ServeHTTP(rw, req)
		res := rw.Result()
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode)
	})
}

func TestServerTailnet_Healthcheck(t *testing.T) {
	t.Parallel()

	// Verifies that a non-nil healthcheck which returns a non-error response behaves as expected.
	t.Run("Passing", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		fn := func(ctx context.Context) error { return nil }

		agents, serverTailnet := setupServerTailnetAgent(t, 1, withHealthcheckFn(fn))

		a := agents[0]
		conn, release, err := serverTailnet.AgentConn(ctx, a.id)
		t.Cleanup(release)
		require.NoError(t, err)
		assert.True(t, conn.AwaitReachable(ctx))
	})

	// If the healthcheck fails, we have no insight into this at this level.
	// The dial against the control plane is retried, so we wait for the context to timeout as an indication that the
	// healthcheck is performing as expected.
	t.Run("Failing", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		fn := func(ctx context.Context) error { return xerrors.Errorf("oops, db gone") }

		agents, serverTailnet := setupServerTailnetAgent(t, 1, withHealthcheckFn(fn))

		a := agents[0]
		_, release, err := serverTailnet.AgentConn(ctx, a.id)
		require.Nil(t, release)
		require.ErrorContains(t, err, "agent is unreachable")
	})
}

type wrappedListener struct {
	net.Listener
	dials int32
}

func (w *wrappedListener) Accept() (net.Conn, error) {
	conn, err := w.Listener.Accept()
	if err != nil {
		return nil, err
	}

	atomic.AddInt32(&w.dials, 1)
	return conn, nil
}

func (w *wrappedListener) getDials() int {
	return int(atomic.LoadInt32(&w.dials))
}

type agentWithID struct {
	id uuid.UUID
	agent.Agent
}

type serverOption struct {
	HealthcheckFn      func(ctx context.Context) error
	DERPAndStunOptions []tailnettest.DERPAndStunOption
}

func withHealthcheckFn(fn func(ctx context.Context) error) serverOption {
	return serverOption{
		HealthcheckFn: fn,
	}
}

func withDERPAndStunOptions(opts ...tailnettest.DERPAndStunOption) serverOption {
	return serverOption{
		DERPAndStunOptions: opts,
	}
}

func setupServerTailnetAgent(t *testing.T, agentNum int, opts ...serverOption) ([]agentWithID, *coderd.ServerTailnet) {
	logger := testutil.Logger(t)

	var healthcheckFn func(ctx context.Context) error
	var derpAndStunOptions []tailnettest.DERPAndStunOption
	for _, opt := range opts {
		derpAndStunOptions = append(derpAndStunOptions, opt.DERPAndStunOptions...)
		if opt.HealthcheckFn != nil {
			healthcheckFn = opt.HealthcheckFn
		}
	}

	derpMap, derpServer := tailnettest.RunDERPAndSTUN(t, derpAndStunOptions...)

	coord := tailnet.NewCoordinator(logger)
	t.Cleanup(func() {
		_ = coord.Close()
	})
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)

	agents := []agentWithID{}

	for i := 0; i < agentNum; i++ {
		manifest := agentsdk.Manifest{
			AgentID: uuid.New(),
			DERPMap: derpMap,
		}

		c := agenttest.NewClient(t, logger, manifest.AgentID, manifest, make(chan *proto.Stats, 50), coord)
		t.Cleanup(c.Close)

		options := agent.Options{
			Client:     c,
			Filesystem: afero.NewMemMapFs(),
			Logger:     logger.Named("agent"),
		}

		ag := agent.New(options)
		t.Cleanup(func() {
			_ = ag.Close()
		})

		// Wait for the agent to connect.
		require.Eventually(t, func() bool {
			return coord.Node(manifest.AgentID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		agents = append(agents, agentWithID{id: manifest.AgentID, Agent: ag})
	}

	dialer := &coderd.InmemTailnetDialer{
		CoordPtr:              &coordPtr,
		DERPFn:                func() *tailcfg.DERPMap { return derpMap },
		Logger:                logger,
		ClientID:              uuid.UUID{5},
		DatabaseHealthcheckFn: healthcheckFn,
	}
	serverTailnet, err := coderd.NewServerTailnet(
		context.Background(),
		logger,
		derpServer,
		dialer,
		false,
		!derpMap.HasSTUN(),
		trace.NewNoopTracerProvider(),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = serverTailnet.Close()
	})

	return agents, serverTailnet
}
