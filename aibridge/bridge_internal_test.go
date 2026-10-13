package aibridge

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/intercept/eventstream"
	"github.com/coder/coder/v2/aibridge/mcpmock"
	"github.com/coder/coder/v2/testutil"
)

// InflightRequests returns the number of admitted requests still running.
func (b *RequestBridge) InflightRequests() int {
	b.inflight.mu.Lock()
	defer b.inflight.mu.Unlock()
	return b.inflight.active
}

// TestRequestBridgeShutdownDoesNotWaitForCanceledHandler verifies shutdown
// returns even when a canceled handler remains blocked.
func TestRequestBridgeShutdownDoesNotWaitForCanceledHandler(t *testing.T) {
	t.Parallel()
	for _, withMCP := range []bool{false, true} {
		name := "WithoutMCP"
		if withMCP {
			name = "WithMCP"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			// Set up a handler that only test cleanup can release.
			started := make(chan context.Context, 1)
			release := make(chan struct{})
			finished := make(chan struct{})
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(_ http.ResponseWriter, r *http.Request) {
				started <- r.Context()
				<-release // Deliberately ignore request cancellation.
			})
			bridge := &RequestBridge{
				inflight: NewInflightGate(slogtest.Make(t, nil)),
				logger:   slogtest.Make(t, nil),
			}
			bridge.handler = bridge.inflight.Middleware(mux)
			// MCP cleanup must be attempted while the handler is still blocked.
			if withMCP {
				proxy := mcpmock.NewMockServerProxier(gomock.NewController(t))
				proxy.EXPECT().Shutdown(gomock.Any()).DoAndReturn(func(shutdownCtx context.Context) error {
					assert.ErrorIs(t, shutdownCtx.Err(), context.DeadlineExceeded)
					assert.EqualValues(t, 1, bridge.InflightRequests())
					return nil
				})
				bridge.mcpProxy = proxy
			}
			t.Cleanup(func() {
				close(release)
				testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, finished)
			})
			go func() {
				defer close(finished)
				bridge.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
			}()
			requestCtx := testutil.TryReceive(ctx, t, started)
			// Expire shutdown while the request is in flight.
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancel()
			shutdownDone := make(chan error, 1)
			go func() { shutdownDone <- bridge.Shutdown(deadlineCtx) }()
			// Shutdown returns and cancels the context without waiting for release.
			require.ErrorIs(t, testutil.TryReceive(ctx, t, shutdownDone), context.DeadlineExceeded)
			testutil.TryReceive(ctx, t, requestCtx.Done())
			require.EqualValues(t, 1, bridge.InflightRequests(), "handler is still blocked")
			// Admission stays closed even though the old handler is still running.
			rejected := httptest.NewRecorder()
			bridge.ServeHTTP(rejected, httptest.NewRequest(http.MethodGet, "/", nil))
			require.Equal(t, http.StatusServiceUnavailable, rejected.Code)
			require.Equal(t, "AI Gateway is shutting down\n", rejected.Body.String())
		})
	}
}

func TestIsClientAbort(t *testing.T) {
	t.Parallel()

	clientCanceled := func(t *testing.T) context.Context {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		return ctx
	}
	shutdownCanceled := func(t *testing.T) context.Context {
		gate := NewInflightGate(slogtest.Make(t, nil))
		ctx, cleanup := gate.RequestContext(t.Context())
		t.Cleanup(cleanup)
		gate.Close()
		return ctx
	}
	timedOut := func(t *testing.T) context.Context {
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		t.Cleanup(cancel)
		return ctx
	}

	cases := []struct {
		name string
		ctx  func(*testing.T) context.Context
		err  error
		want bool
	}{
		{"Succeeded", clientCanceled, nil, false},
		{"ClientCanceled", clientCanceled, xerrors.Errorf("relay chunk: %w", context.Canceled), true},
		{"ClientDisconnected", clientCanceled, xerrors.Errorf("relay event: %w", eventstream.ErrEventStreamClosed), true},
		{"BrokenConnection", clientCanceled, xerrors.Errorf("read body: %w", net.ErrClosed), true},
		// An upstream error keeps its category when the client leaves after
		// receiving it.
		{"UpstreamErrorThenClientLeft", clientCanceled, xerrors.New("rate limited"), false},
		{"ClientStillConnected", func(t *testing.T) context.Context { return t.Context() }, context.Canceled, false},
		{"GatewayShutdown", shutdownCanceled, context.Canceled, false},
		{"Timeout", timedOut, context.DeadlineExceeded, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, isClientAbort(tc.ctx(t), tc.err))
		})
	}
}
