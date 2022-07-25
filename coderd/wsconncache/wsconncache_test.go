package wsconncache_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/wsconncache"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestCache(t *testing.T) {
	t.Parallel()
	t.Run("Same", func(t *testing.T) {
		t.Parallel()
		cache := wsconncache.New(func(r *http.Request, id uuid.UUID) (*agent.Conn, error) {
			return setupAgent(t, agent.Metadata{}, 0), nil
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
		cache := wsconncache.New(func(r *http.Request, id uuid.UUID) (*agent.Conn, error) {
			called.Add(1)
			return setupAgent(t, agent.Metadata{}, 0), nil
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
		cache := wsconncache.New(func(r *http.Request, id uuid.UUID) (*agent.Conn, error) {
			return setupAgent(t, agent.Metadata{}, 0), nil
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
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
		}
		defer func() {
			_ = server.Close()
		}()
		go server.Serve(random)

		cache := wsconncache.New(func(r *http.Request, id uuid.UUID) (*agent.Conn, error) {
			return setupAgent(t, agent.Metadata{}, 0), nil
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
				res.Result().Body.Close()
				assert.Equal(t, http.StatusOK, res.Result().StatusCode)
			}()
		}
		wg.Wait()
	})
}

func setupAgent(t *testing.T, metadata agent.Metadata, ptyTimeout time.Duration) *agent.Conn {
	client, server := provisionersdk.TransportPipe()
	closer := agent.New(func(ctx context.Context, logger slog.Logger) (agent.Metadata, *peerbroker.Listener, error) {
		listener, err := peerbroker.Listen(server, func(ctx context.Context) ([]webrtc.ICEServer, *peer.ConnOptions, error) {
			return nil, &peer.ConnOptions{
				Logger: slogtest.Make(t, nil).Named("server").Leveled(slog.LevelDebug),
			}, nil
		})
		return metadata, listener, err
	}, &agent.Options{
		Logger:                 slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
		ReconnectingPTYTimeout: ptyTimeout,
	})
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		_ = closer.Close()
	})
	api := proto.NewDRPCPeerBrokerClient(provisionersdk.Conn(client))
	stream, err := api.NegotiateConnection(context.Background())
	assert.NoError(t, err)
	conn, err := peerbroker.Dial(stream, []webrtc.ICEServer{}, &peer.ConnOptions{
		Logger: slogtest.Make(t, nil).Named("client").Leveled(slog.LevelDebug),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	return &agent.Conn{
		Negotiator: api,
		Conn:       conn,
	}
}
