package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
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
	codertestutil "github.com/coder/coder/v2/testutil"
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
			handler, err := newForwardingHandler(prov, logger, nil, noop.NewTracerProvider().Tracer(t.Name()), gate, &struct{ recorder.Recorder }{}, http.DefaultTransport)
			require.NoError(t, err)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			require.Equal(t, tc.status, response.Code)
			require.Contains(t, response.Body.String(), tc.message)
			require.False(t, body.read, "placeholder must not read the request body")
			require.False(t, body.closed, "the transport owns request-body cleanup")
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			require.NoError(t, gate.Shutdown(ctx), "completed requests must release their admission")
		})
	}
}

func requestWithAuth(t *testing.T, body io.Reader) *http.Request {
	t.Helper()
	ctx := aibcontext.AsActor(t.Context(), aibcontext.Actor{
		ID:       uuid.New(),
		APIKeyID: uuid.NewString(),
		Username: t.Name(),
		Email:    "actor@example.test",
	})
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", body).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer user-secret-key")
	return req
}

func TestForwardingHandlerRejectsRequest(t *testing.T) {
	t.Parallel()
	resolutionErr := xerrors.New("credential store unavailable")
	for _, tc := range []struct {
		name           string
		prepare        func(*testing.T, *http.Request) *http.Request
		provider       func(*testing.T) provider.Provider
		noRecorder     bool
		status         int
		wantBody       string
		wantLogLevel   slog.Level
		wantLogMessage string
		wantLogError   error
	}{
		{
			name: "WebSocket",
			prepare: func(_ *testing.T, r *http.Request) *http.Request {
				r.Method = http.MethodGet
				r.Header.Set("Connection", "Upgrade")
				r.Header.Set("Upgrade", "websocket")
				return r
			},
			status:         http.StatusNotImplemented,
			wantBody:       "WebSocket transport is not supported, use HTTP\n",
			wantLogLevel:   slog.LevelDebug,
			wantLogMessage: "rejecting unsupported WebSocket upgrade",
		},
		{
			name: "MalformedFirewallSessionID",
			prepare: func(_ *testing.T, r *http.Request) *http.Request {
				r.Header.Set("X-Coder-Agent-Firewall-Session-Id", "not-a-uuid")
				r.Header.Set("X-Coder-Agent-Firewall-Sequence-Number", "1")
				return r
			},
			status:         http.StatusBadRequest,
			wantBody:       "invalid agent firewall headers\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "rejecting request with invalid agent firewall headers",
		},
		{
			name: "FirewallMissingSequence",
			prepare: func(_ *testing.T, r *http.Request) *http.Request {
				r.Header.Set("X-Coder-Agent-Firewall-Session-Id", "e5f6a7b8-1234-5678-9abc-def012345678")
				return r
			},
			status:         http.StatusBadRequest,
			wantBody:       "invalid agent firewall headers\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "rejecting request with invalid agent firewall headers",
		},
		{
			name: "DeclaredOversize",
			prepare: func(_ *testing.T, r *http.Request) *http.Request {
				r.ContentLength = routing.MaxRequestBodyBytes + 1
				return r
			},
			status:         http.StatusRequestEntityTooLarge,
			wantBody:       fmt.Sprintf("Request body too large. The maximum allowed request body size is %dMiB.\n", routing.MaxRequestBodyBytes>>20),
			wantLogLevel:   slog.LevelDebug,
			wantLogMessage: "rejecting oversized request body",
		},
		{
			name:           "MissingActor",
			prepare:        func(t *testing.T, r *http.Request) *http.Request { return r.WithContext(t.Context()) },
			status:         http.StatusBadRequest,
			wantBody:       "no actor found\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "rejecting request without an actor",
		},
		{
			name:           "MissingRecorder",
			noRecorder:     true,
			status:         http.StatusInternalServerError,
			wantBody:       "recorder unavailable\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "rejecting request without a recorder",
		},
		{
			name: "MissingCredential",
			prepare: func(_ *testing.T, r *http.Request) *http.Request {
				r.Header.Del("Authorization")
				return r
			},
			status:         http.StatusForbidden,
			wantBody:       "upstream authentication unavailable: no provider credentials supplied or configured\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "failed to resolve credential",
		},
		{
			name: "MissingCopilotCredential",
			prepare: func(_ *testing.T, r *http.Request) *http.Request {
				r.Header.Del("Authorization")
				return r
			},
			provider: func(*testing.T) provider.Provider {
				return provider.NewCopilot(config.Copilot{BaseURL: "https://upstream.example.test"})
			},
			status:         http.StatusForbidden,
			wantBody:       "upstream authentication unavailable: no provider credentials supplied or configured\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "failed to resolve credential",
		},
		{
			name: "CredentialResolutionError",
			provider: func(*testing.T) provider.Provider {
				return credentialErrorProvider{
					Provider: provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test"}),
					err:      resolutionErr,
				}
			},
			status:         http.StatusInternalServerError,
			wantBody:       "upstream authentication unavailable\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "failed to resolve credential",
			wantLogError:   resolutionErr,
		},
		{
			name: "UnsupportedSigning",
			prepare: func(_ *testing.T, r *http.Request) *http.Request {
				r.Header.Del("Authorization")
				return r
			},
			provider: func(t *testing.T) provider.Provider {
				prov, err := provider.NewAnthropic(t.Context(), config.Anthropic{BaseURL: "https://upstream.example.test"}, nil, &config.AWSClaudePlatform{Region: "us-west-2", WorkspaceID: "wrkspc_test"})
				require.NoError(t, err)
				return prov
			},
			status:         http.StatusNotImplemented,
			wantBody:       "upstream authentication is not supported in proxy mode\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "rejecting unsupported upstream credential",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var rec recorder.Recorder = &struct{ recorder.Recorder }{}
			if tc.noRecorder {
				rec = nil
			}
			body := &unreadBody{}
			req := requestWithAuth(t, body)
			if tc.prepare != nil {
				req = tc.prepare(t, req)
			}
			var prov provider.Provider = provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test"})
			if tc.provider != nil {
				prov = tc.provider(t)
			}
			sink := codertestutil.NewFakeSink(t)
			h := &forwardingHandler{provider: prov, logger: sink.Logger(), recorder: rec}
			response := httptest.NewRecorder()
			record, cred := h.checkRequest(response, req)
			require.Nil(t, record)
			require.Nil(t, cred)
			require.Equal(t, tc.status, response.Code)
			require.Equal(t, tc.wantBody, response.Body.String())
			require.False(t, body.read, "preflight must not read the request body")
			require.False(t, body.closed, "the transport owns request-body cleanup")
			entries := sink.Entries()
			require.Len(t, entries, 1)
			require.Equal(t, tc.wantLogLevel, entries[0].Level)
			require.Equal(t, tc.wantLogMessage, entries[0].Message)
			require.Contains(t, entries[0].Fields, slog.F("provider", prov.Name()))
			if tc.wantLogError != nil {
				require.Contains(t, entries[0].Fields, slog.Error(tc.wantLogError))
				require.Contains(t, entries[0].Fields, slog.F("path", req.URL.Path))
			}
			require.NotContains(t, fmt.Sprint(entries), "user-secret-key", "rejection logs must not include credentials")
		})
	}
}

type credentialErrorProvider struct {
	provider.Provider
	err error
}

func (p credentialErrorProvider) ResolveCredential(*http.Request) (credential.Credential, error) {
	return nil, p.err
}

func TestForwardingHandlerInterceptionRecord(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		useKeyPool bool
		wantKind   credential.Kind
		wantHint   string
	}{
		{
			name:     "BYOK",
			wantKind: credential.KindBYOK,
			wantHint: utils.MaskSecret("user-secret-key"),
		},
		{
			name:       "KeyPool",
			useKeyPool: true,
			wantKind:   credential.KindCentralized,
			wantHint:   credential.HintFailoverKey,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := &unreadBody{}
			req := requestWithAuth(t, body)
			req.Header.Set("User-Agent", "claude-code/1.0")
			req.Header.Set("X-Coder-Agent-Firewall-Session-Id", "e5f6a7b8-1234-5678-9abc-def012345678")
			req.Header.Set("X-Coder-Agent-Firewall-Sequence-Number", "42")
			var pool *keypool.Pool
			if tc.useKeyPool {
				var err error
				pool, err = keypool.New("openai", []string{"pool-key"}, quartz.NewMock(t), nil)
				require.NoError(t, err)
				req.Header.Del("Authorization")
			}
			h := &forwardingHandler{
				provider: provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test", KeyPool: pool}),
				recorder: &struct{ recorder.Recorder }{},
			}
			response := httptest.NewRecorder()
			before := time.Now()
			record, cred := h.checkRequest(response, req)
			require.NotNil(t, record)
			require.NotNil(t, cred)
			id, err := uuid.Parse(record.ID)
			require.NoError(t, err)
			require.NotEqual(t, uuid.Nil, id)
			require.WithinRange(t, record.StartedAt, before, time.Now())
			require.Equal(t, time.UTC, record.StartedAt.Location())
			require.Equal(t, tc.wantKind, cred.Kind())
			require.Equal(t, tc.wantKind, record.CredentialKind)
			require.Equal(t, tc.wantHint, record.CredentialHint)
			require.Equal(t, aibcontext.ActorIDFromContext(req.Context()), record.InitiatorID)
			require.Equal(t, recorder.Metadata{"Username": t.Name()}, record.Metadata)
			require.Equal(t, "openai", record.ProviderName)
			require.Equal(t, config.ProviderOpenAI, record.Provider)
			require.Equal(t, string(client.ClaudeCode), record.Client)
			require.Equal(t, req.UserAgent(), record.UserAgent)
			require.Equal(t, new("e5f6a7b8-1234-5678-9abc-def012345678"), record.AgentFirewallSessionID)
			require.Equal(t, new(int32(42)), record.AgentFirewallSequenceNumber)
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
	handler, err := newForwardingHandler(prov, logger, nil, noop.NewTracerProvider().Tracer(t.Name()), gate, &struct{ recorder.Recorder }{}, http.DefaultTransport)
	require.NoError(t, err)
	const route = "/v1/responses"
	require.NoError(t, handler.breaker.Execute(route, "", httptest.NewRecorder(), func(w http.ResponseWriter) error {
		w.WriteHeader(http.StatusServiceUnavailable)
		return nil
	}))

	for _, tc := range []struct {
		name         string
		websocket    bool
		noCredential bool
		status       int
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
		{
			name:         "CredentialBeforeCircuit",
			noCredential: true,
			status:       http.StatusForbidden,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := &unreadBody{}
			req := httptest.NewRequest(http.MethodPost, prov.RoutePrefix()+"/responses", body)
			req = req.WithContext(aibridge.AsActor(t.Context(), aibridge.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()}))
			req.Header.Set("Authorization", "Bearer user-key")
			if tc.noCredential {
				req.Header.Del("Authorization")
			}
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
			if tc.status == http.StatusServiceUnavailable {
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
