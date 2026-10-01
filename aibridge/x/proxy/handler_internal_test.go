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
	"github.com/coder/coder/v2/aibridge/client"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/credential"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/quartz"
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
		{name: "UnparsedBody", status: http.StatusNotImplemented, message: "bridged routes are not yet implemented in proxy mode"},
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

func recordedRequest(t *testing.T, body io.Reader) *http.Request {
	t.Helper()
	ctx := aibcontext.AsActor(t.Context(), aibcontext.Actor{
		ID:       uuid.New(),
		APIKeyID: uuid.NewString(),
		Username: t.Name(),
		Email:    "actor@example.test",
	})
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", body).WithContext(ctx)
	req.Pattern = "/openai/v1/chat/completions"
	req.Header.Set("Authorization", "Bearer user-secret-key")
	return req
}

func TestForwardingHandlerRejectsRequest(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"MissingActor", "MissingRecorder", "WebSocket", "Firewall", "DeclaredOversize", "MissingCredential", "UnsupportedSigning"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var rec recorder.Recorder = &struct{ recorder.Recorder }{}
			body := &unreadBody{}
			req := recordedRequest(t, body)
			var prov provider.Provider = provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test"})
			status := http.StatusBadRequest
			switch name {
			case "MissingActor":
				req = req.WithContext(t.Context())
			case "MissingRecorder":
				rec = nil
				status = http.StatusInternalServerError
			case "WebSocket":
				req.Method = http.MethodGet
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", "websocket")
				status = http.StatusNotImplemented
			case "Firewall":
				req.Header.Set("X-Coder-Agent-Firewall-Session-Id", "invalid")
			case "DeclaredOversize":
				req.ContentLength = routing.MaxRequestBodyBytes + 1
				status = http.StatusRequestEntityTooLarge
			case "MissingCredential":
				req.Header.Del("Authorization")
				status = http.StatusBadGateway
			case "UnsupportedSigning":
				var err error
				prov, err = provider.NewAnthropic(t.Context(), config.Anthropic{BaseURL: "https://upstream.example.test"}, nil, &config.AWSClaudePlatform{Region: "us-west-2", WorkspaceID: "wrkspc_test"})
				require.NoError(t, err)
				req.Header.Del("Authorization")
				status = http.StatusNotImplemented
			}
			h := &forwardingHandler{provider: prov, logger: slogtest.Make(t, nil), recorder: rec}
			response := httptest.NewRecorder()
			record, cred := h.checkRequest(response, req)
			require.Nil(t, record)
			require.Nil(t, cred)
			require.Equal(t, status, response.Code)
			require.False(t, body.read, "preflight must not read the request body")
			require.Equal(t, name == "DeclaredOversize", body.closed)
		})
	}
}

func TestForwardingHandlerRequestMetadata(t *testing.T) {
	t.Parallel()
	for _, pooled := range []bool{false, true} {
		name := "BYOK"
		if pooled {
			name = "Pool"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body := &unreadBody{}
			req := recordedRequest(t, body)
			req.Header.Set("User-Agent", "claude-code/1.0")
			req.Header.Set("X-Coder-Agent-Firewall-Session-Id", "e5f6a7b8-1234-5678-9abc-def012345678")
			req.Header.Set("X-Coder-Agent-Firewall-Sequence-Number", "42")
			var pool *keypool.Pool
			kind, hint := credential.KindBYOK, utils.MaskSecret("user-secret-key")
			if pooled {
				var err error
				pool, err = keypool.New("openai", []string{"pool-key"}, quartz.NewMock(t), nil)
				require.NoError(t, err)
				req.Header.Del("Authorization")
				kind, hint = credential.KindCentralized, credential.HintFailoverKey
			}
			h := &forwardingHandler{
				provider: provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test", KeyPool: pool}),
				recorder: &struct{ recorder.Recorder }{},
			}
			response := httptest.NewRecorder()
			record, cred := h.checkRequest(response, req)
			require.NotNil(t, record)
			require.NotNil(t, cred)
			require.Equal(t, kind, cred.Kind())
			require.Equal(t, kind, record.CredentialKind)
			require.Equal(t, hint, record.CredentialHint)
			require.Equal(t, aibcontext.ActorIDFromContext(req.Context()), record.InitiatorID)
			require.Equal(t, recorder.Metadata{"Username": t.Name()}, record.Metadata)
			require.Equal(t, "openai", record.ProviderName)
			require.Equal(t, string(client.ClaudeCode), record.Client)
			require.Equal(t, req.UserAgent(), record.UserAgent)
			require.Equal(t, new("e5f6a7b8-1234-5678-9abc-def012345678"), record.AgentFirewallSessionID)
			require.Equal(t, new(int32(42)), record.AgentFirewallSequenceNumber)
			require.Empty(t, record.Model)
			require.Empty(t, response.Body.String())
			require.False(t, body.read)
			require.False(t, body.closed)
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
