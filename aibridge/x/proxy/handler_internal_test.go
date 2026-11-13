package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/coder/v2/aibridge/x/proxy/replaybody"
	codertestutil "github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type credentialErrorProvider struct {
	provider.Provider
	err error
}

func (p credentialErrorProvider) ResolveCredential(*http.Request) (credential.Credential, error) {
	return nil, p.err
}

// roundTripFunc adapts a function to http.RoundTripper so tests can observe
// each upstream attempt.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// trackedBody counts how often a body reaches EOF and is closed.
type trackedBody struct {
	io.Reader
	closes, eofs atomic.Int32
}

func (b *trackedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF { //nolint:errorlint // Track actual stream completion.
		b.eofs.Add(1)
	}
	return n, err
}

func (b *trackedBody) Close() error {
	b.closes.Add(1)
	if closer, ok := b.Reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
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

// newTestForwardingHandler constructs a forwarding handler and shuts down its
// inflight gate after the test.
func newTestForwardingHandler(t *testing.T, prov provider.Provider, m *metrics.Metrics) *forwardingHandler {
	t.Helper()
	logger := slogtest.Make(t, nil)
	gate := aibridge.NewInflightGate(logger)
	t.Cleanup(func() {
		defer gate.Close()
		require.NoError(t, gate.Shutdown(codertestutil.Context(t, codertestutil.WaitLong)))
	})
	h, err := newForwardingHandler(prov, logger, m, noop.NewTracerProvider().Tracer(t.Name()), gate, &struct{ recorder.Recorder }{})
	require.NoError(t, err)
	return h
}

// newTestPool returns a key pool that offers keys in the given order.
func newTestPool(t *testing.T, keys ...string) *keypool.Pool {
	t.Helper()
	pool, err := keypool.New("test", keys, quartz.NewMock(t), nil)
	require.NoError(t, err)
	return pool
}

// prepareAndProxy validates r, captures its body for replay, and serves it
// through a proxy bound to the resolved credential, without response
// observation.
func prepareAndProxy(t *testing.T, h *forwardingHandler, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	_, record, cred := h.checkRequest(w, r)
	require.NotNil(t, record, "request must pass validation")
	outbound, body := replaybody.NewRequest(r)
	t.Cleanup(func() { _ = body.Close() })
	h.newProxy(cred).ServeHTTP(w, outbound)
	require.NoError(t, body.Close())
	_, err := body.Wait(codertestutil.Context(t, codertestutil.WaitLong))
	require.NoError(t, err)
}

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
			handler, err := newForwardingHandler(prov, logger, nil, noop.NewTracerProvider().Tracer(t.Name()), gate, &testutil.MockRecorder{})
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

func TestCheckRequest(t *testing.T) {
	t.Parallel()

	t.Run("Rejects", func(t *testing.T) {
		t.Parallel()
		resolutionErr := xerrors.New("credential store unavailable")
		for _, tc := range []struct {
			name           string
			prepare        func(*testing.T, *http.Request) *http.Request
			provider       func(*testing.T) provider.Provider
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
				h, err := newForwardingHandler(prov, sink.Logger(), nil, nil, nil, &testutil.MockRecorder{})
				require.NoError(t, err)
				response := httptest.NewRecorder()
				_, record, cred := h.checkRequest(response, req)
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
	})

	t.Run("Record", func(t *testing.T) {
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
				}
				response := httptest.NewRecorder()
				before := time.Now()
				_, record, cred := h.checkRequest(response, req)
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
	})
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
	handler, err := newForwardingHandler(prov, logger, nil, noop.NewTracerProvider().Tracer(t.Name()), gate, &testutil.MockRecorder{})
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

// Forwarding replaces the route prefix with the provider base URL and keeps the
// client's escaped path and raw query bytes.
func TestForwardingRewritesURL(t *testing.T) {
	t.Parallel()
	requestURI := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requestURI <- r.RequestURI
	}))
	t.Cleanup(upstream.Close)
	h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL + "/base?configured=1"}), nil)
	req := requestWithAuth(t, nil)
	req.URL.Path = "/openai/v1/models/a/b"
	req.URL.RawPath = "/openai/v1/models/a%2Fb"
	req.URL.RawQuery = "raw=a;b"

	response := httptest.NewRecorder()
	prepareAndProxy(t, h, response, req)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "/base/models/a%2Fb?configured=1&raw=a;b", codertestutil.RequireReceive(codertestutil.Context(t, codertestutil.WaitShort), t, requestURI))
}

// The client's Accept-Encoding never reaches upstream. The transport negotiates
// gzip and decodes it, so the client receives an uncompressed body.
func TestForwardingDecodesResponses(t *testing.T) {
	t.Parallel()
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	_, err := gz.Write([]byte("raw response"))
	require.NoError(t, err)
	require.NoError(t, gz.Close())

	for _, tc := range []struct {
		name     string
		encoding string
	}{
		{name: "NoClientEncoding"}, // gzip is the default for Go clients
		{name: "ClientBrotli", encoding: "br"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			encoding := make(chan string, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				encoding <- r.Header.Get("Accept-Encoding")
				w.Header().Set("Content-Encoding", "gzip")
				_, _ = w.Write(compressed.Bytes())
			}))
			t.Cleanup(upstream.Close)
			h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL}), nil)
			req := requestWithAuth(t, nil)
			if tc.encoding != "" {
				req.Header.Set("Accept-Encoding", tc.encoding)
			}

			response := httptest.NewRecorder()
			prepareAndProxy(t, h, response, req)
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, "gzip", codertestutil.RequireReceive(codertestutil.Context(t, codertestutil.WaitShort), t, encoding))
			require.Equal(t, "raw response", response.Body.String())
			require.Empty(t, response.Header().Get("Content-Encoding"))
		})
	}
}

// Forwarding dumps each upstream exchange under the provider's API dump
// directory. The apidump package owns the dump contents and redaction.
func TestForwardingAPIDump(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(upstream.Close)
	dumpDir := t.TempDir()
	h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL, APIDumpDir: dumpDir}), nil)

	response := httptest.NewRecorder()
	prepareAndProxy(t, h, response, requestWithAuth(t, nil))
	require.Equal(t, http.StatusOK, response.Code)
	for _, suffix := range []string{apidump.SuffixRequest, apidump.SuffixResponse} {
		files, err := filepath.Glob(filepath.Join(dumpDir, "openai", "passthrough", "*"+suffix))
		require.NoError(t, err)
		require.Len(t, files, 1, suffix)
	}
}

// Forwarding sends the BYOK credential the provider resolved, in the header it
// came from. A BYOK key failure reaches the client without falling back to
// pooled keys.
func TestForwardingProviderCredentials(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		provider func(t *testing.T, baseURL string, pool *keypool.Pool) provider.Provider
		// header is added to the request's BYOK Authorization header.
		header   http.Header
		wantAuth http.Header
	}{
		{
			name: "OpenAIConnectionAuth",
			provider: func(_ *testing.T, baseURL string, pool *keypool.Pool) provider.Provider {
				return provider.NewOpenAI(config.OpenAI{BaseURL: baseURL, KeyPool: pool})
			},
			// ReverseProxy removes headers nominated by Connection.
			// Handler must restore the BYOK credential.
			header:   http.Header{"Connection": {"Authorization"}},
			wantAuth: http.Header{"Authorization": {"Bearer user-secret-key"}},
		},
		{
			name: "AnthropicAPIKey",
			provider: func(t *testing.T, baseURL string, pool *keypool.Pool) provider.Provider {
				prov, err := provider.NewAnthropic(t.Context(), config.Anthropic{BaseURL: baseURL, KeyPool: pool}, nil, nil)
				require.NoError(t, err)
				return prov
			},
			// Anthropic prefers X-Api-Key, Authorization header is dropped.
			header:   http.Header{"X-Api-Key": {"anthropic-secret-key"}},
			wantAuth: http.Header{"X-Api-Key": {"anthropic-secret-key"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			received := make(chan http.Header, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- r.Header.Clone()
				w.WriteHeader(http.StatusUnauthorized)
			}))
			t.Cleanup(upstream.Close)
			pool := newTestPool(t, "pool-secret-key")
			prov := tc.provider(t, upstream.URL, pool)
			req := requestWithAuth(t, nil)
			req.URL.Path = prov.RoutePrefix() + "/messages"
			for name, values := range tc.header {
				req.Header[name] = values
			}
			h := newTestForwardingHandler(t, prov, nil)

			response := httptest.NewRecorder()
			prepareAndProxy(t, h, response, req)
			require.Equal(t, []keypool.KeyState{keypool.KeyStateValid}, pool.PoolState(), "BYOK requests must not use pooled keys")
			require.Equal(t, http.StatusUnauthorized, response.Code)
			got := codertestutil.RequireReceive(codertestutil.Context(t, codertestutil.WaitShort), t, received)
			for _, header := range []string{"Authorization", "X-Api-Key"} {
				require.Equal(t, tc.wantAuth.Values(header), got.Values(header), header)
			}
		})
	}
}

// Providers that adjust upstream requests themselves apply to forwarded
// requests after key failover selects a pooled key. Claude Platform sets its
// configured workspace.
func TestForwardingProviderTransport(t *testing.T) {
	t.Parallel()
	received := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
	}))
	t.Cleanup(upstream.Close)
	prov, err := provider.NewAnthropic(t.Context(), config.Anthropic{BaseURL: upstream.URL, KeyPool: newTestPool(t, "pool-secret-key")}, nil, &config.AWSClaudePlatform{Region: "us-west-2", BaseURL: upstream.URL, WorkspaceID: "wrkspc_config"})
	require.NoError(t, err)
	h := newTestForwardingHandler(t, prov, nil)
	req := requestWithAuth(t, nil)
	req.URL.Path = prov.RoutePrefix() + "/messages"
	req.Header.Del("Authorization")

	response := httptest.NewRecorder()
	prepareAndProxy(t, h, response, req)
	require.Equal(t, http.StatusOK, response.Code)
	got := codertestutil.RequireReceive(codertestutil.Context(t, codertestutil.WaitShort), t, received)
	require.Equal(t, "wrkspc_config", got.Get("Anthropic-Workspace-Id"))
	require.Equal(t, "pool-secret-key", got.Get("X-Api-Key"), "request signing must not replace the pooled key")
}

// Per-request proxies share the handler's transport.
func TestForwardingReusesUpstreamConnection(t *testing.T) {
	t.Parallel()
	type upstreamRequest struct{ auth, remoteAddr string }
	requests := make(chan upstreamRequest, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requests <- upstreamRequest{auth: r.Header.Get("Authorization"), remoteAddr: r.RemoteAddr}
	}))
	t.Cleanup(upstream.Close)
	h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL}), nil)

	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	send := func(key string) string {
		t.Helper()
		req := requestWithAuth(t, nil)
		req.Header.Set("Authorization", "Bearer "+key)
		response := httptest.NewRecorder()
		prepareAndProxy(t, h, response, req)
		require.Equal(t, http.StatusOK, response.Code)
		got := codertestutil.RequireReceive(ctx, t, requests)
		require.Equal(t, "Bearer "+key, got.auth, "each request must use its own credential")
		return got.remoteAddr
	}
	first := send("first-key")
	require.Equal(t, first, send("second-key"), "per-request proxies must reuse the upstream connection")
}

// Centralized requests move to the next key after a key failure. The first
// attempt forwards the body as it is read, so the retry replays the consumed
// prefix and then continues reading the input. The rejected response is
// drained and closed without reaching the client.
func TestForwardingFailoverRetry(t *testing.T) {
	t.Parallel()
	const prefix = `{"metadata":{"user_id":`
	const suffix = `"foobar"}}`
	input := strings.NewReader(prefix + suffix)
	h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test", KeyPool: newTestPool(t, "first-key", "second-key")}), nil)
	rejected := &trackedBody{Reader: strings.NewReader("rejected key")}
	var auth []string
	h.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		defer r.Body.Close()
		auth = append(auth, r.Header.Get("Authorization"))
		if len(auth) == 1 {
			chunk := make([]byte, len(prefix))
			_, err := io.ReadFull(r.Body, chunk)
			assert.NoError(t, err)
			assert.Equal(t, prefix, string(chunk))
			assert.Equal(t, len(suffix), input.Len(), "the first attempt must forward the prefix before the input is read")
			return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: rejected}, nil
		}
		payload, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, prefix+suffix, string(payload), "the retry must replay the prefix and read the remaining input")
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	req := requestWithAuth(t, input)
	req.Header.Del("Authorization")
	response := httptest.NewRecorder()
	prepareAndProxy(t, h, response, req)

	require.Equal(t, []string{"Bearer first-key", "Bearer second-key"}, auth)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "ok", response.Body.String())
	require.EqualValues(t, 1, rejected.eofs.Load(), "rejected responses must be drained")
	require.EqualValues(t, 1, rejected.closes.Load(), "rejected responses must be closed")
}
