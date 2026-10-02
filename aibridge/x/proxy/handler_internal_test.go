package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
)

func TestForwardingHandlerPlaceholder(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("placeholder must not call upstream")
	}))
	t.Cleanup(upstream.Close)
	prov := &testutil.MockProvider{
		NameStr: "test", URL: upstream.URL,
		InterceptorFunc: func(http.ResponseWriter, *http.Request, trace.Tracer) (intercept.Interceptor, error) {
			t.Error("placeholder must not create an interceptor")
			return nil, io.ErrUnexpectedEOF
		},
	}
	for _, tc := range []struct {
		name    string
		prepare func(*http.Request)
		status  int
		message string
	}{
		{name: "UnparsedBody", status: http.StatusNotFound, message: "404 page not found"},
		{name: "WebSocket", prepare: func(r *http.Request) {
			r.Method = http.MethodGet
			r.Header.Set("Connection", "Upgrade")
			r.Header.Set("Upgrade", "websocket")
		}, status: http.StatusNotImplemented, message: "WebSocket transport is not supported, use HTTP"},
		{name: "InvalidFirewall", prepare: func(r *http.Request) {
			r.Header.Set("X-Coder-Agent-Firewall-Session-Id", "not-a-uuid")
			r.Header.Set("X-Coder-Agent-Firewall-Sequence-Number", "1")
		}, status: http.StatusBadRequest, message: "invalid agent firewall headers"},
		{name: "DeclaredOversize", prepare: func(r *http.Request) {
			r.ContentLength = routing.MaxRequestBodyBytes + 1
		}, status: http.StatusRequestEntityTooLarge, message: "Request body too large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := &unreadBody{}
			req := httptest.NewRequest(http.MethodPost, "/test/messages", body)
			req = req.WithContext(aibridge.AsActor(t.Context(), aibridge.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()}))
			req.Header.Set("Authorization", "Bearer user-key")
			req.Header.Set("Content-Type", "application/json")
			if tc.prepare != nil {
				tc.prepare(req)
			}
			req.Pattern = "/test/messages"
			logger := slogtest.Make(t, nil)
			gate := aibridge.NewInflightGate(logger)
			t.Cleanup(gate.Close)
			handler := newForwardingHandler(prov, logger, nil, noop.NewTracerProvider().Tracer(t.Name()), gate, &struct{ recorder.Recorder }{})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			require.Equal(t, tc.status, response.Code)
			require.Contains(t, response.Body.String(), tc.message)
			require.False(t, body.read, "placeholder must not read the request body")
			if tc.status == http.StatusRequestEntityTooLarge {
				require.True(t, body.closed)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			require.NoError(t, gate.Shutdown(ctx), "completed requests must release their admission")
		})
	}
}

func TestForwardingHandlerOpenCircuit(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultCircuitBreaker()
	cfg.FailureThreshold = 1
	cfg.Timeout = time.Hour
	prov := provider.NewOpenAI(config.OpenAI{CircuitBreaker: &cfg})
	logger := slogtest.Make(t, nil)
	gate := aibridge.NewInflightGate(logger)
	t.Cleanup(gate.Close)
	handler := newForwardingHandler(prov, logger, nil, noop.NewTracerProvider().Tracer(t.Name()), gate, &struct{ recorder.Recorder }{})
	const route = "/v1/responses"
	require.NoError(t, handler.breaker.Execute(route, "", httptest.NewRecorder(), func(w http.ResponseWriter) error {
		w.WriteHeader(http.StatusServiceUnavailable)
		return nil
	}))

	for _, tc := range []struct {
		name      string
		websocket bool
		status    int
	}{
		{
			name:   "OpenCircuit",
			status: http.StatusServiceUnavailable,
		},
		{
			name:      "ValidationBeforeCircuit",
			websocket: true,
			status:    http.StatusNotImplemented,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := &unreadBody{}
			req := httptest.NewRequest(http.MethodPost, prov.RoutePrefix()+"/responses", body)
			req = req.WithContext(aibridge.AsActor(t.Context(), aibridge.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()}))
			req.Header.Set("Authorization", "Bearer user-key")
			req.Pattern = prov.RoutePrefix() + "/responses"
			if tc.websocket {
				req.Method = http.MethodGet
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", "websocket")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			require.Equal(t, tc.status, response.Code)
			require.False(t, body.read)
			require.False(t, body.closed)
			if !tc.websocket {
				require.Contains(t, response.Body.String(), circuitbreaker.ErrCircuitOpen.Error())
			}
		})
	}
}

type unreadBody struct {
	read   bool
	closed bool
}

func (b *unreadBody) Read([]byte) (int, error) {
	b.read = true
	return 0, io.ErrUnexpectedEOF
}

func (b *unreadBody) Close() error {
	b.closed = true
	return nil
}
