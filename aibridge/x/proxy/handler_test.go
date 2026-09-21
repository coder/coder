package proxy_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/x/proxy"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/quartz"
)

type proxyTestProvider struct {
	*testutil.MockProvider
	typ          string
	authHeader   string
	pool         *keypool.Pool
	failover     keypool.KeyFailoverConfig
	breaker      *config.CircuitBreaker
	failoverCall func(slog.Logger) keypool.KeyFailoverConfig
}

func (p *proxyTestProvider) Type() string           { return p.typ }
func (p *proxyTestProvider) AuthHeader() string     { return p.authHeader }
func (p *proxyTestProvider) KeyPool() *keypool.Pool { return p.pool }
func (p *proxyTestProvider) KeyFailoverConfig(logger slog.Logger) keypool.KeyFailoverConfig {
	if p.failoverCall != nil {
		return p.failoverCall(logger)
	}
	return p.failover
}
func (p *proxyTestProvider) CircuitBreakerConfig() *config.CircuitBreaker { return p.breaker }
func (*proxyTestProvider) CreateInterceptor(http.ResponseWriter, *http.Request, trace.Tracer) (intercept.Interceptor, error) {
	panic("proxy routes must not create interceptors")
}

type failingStartRecorder struct {
	testutil.MockRecorder
	err error
}

func (r *failingStartRecorder) RecordInterception(context.Context, *recorder.InterceptionRecord) error {
	return r.err
}

type closeTrackingBody struct {
	io.Reader
	closed bool
}

func (b *closeTrackingBody) Close() error {
	b.closed = true
	return nil
}

type countingRecorder struct {
	mu      sync.Mutex
	started chan struct{}
	release chan struct{}
	starts  int
	ends    int
	lastID  string
	lastEnd *recorder.InterceptionRecordEnded
}

func (r *countingRecorder) RecordInterception(_ context.Context, record *recorder.InterceptionRecord) error {
	r.mu.Lock()
	r.starts++
	r.lastID = record.ID
	if r.started != nil {
		close(r.started)
		r.started = nil
	}
	r.mu.Unlock()
	if r.release != nil {
		<-r.release
	}
	return nil
}

func (r *countingRecorder) RecordInterceptionEnded(_ context.Context, record *recorder.InterceptionRecordEnded) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ends++
	ended := *record
	r.lastEnd = &ended
	if record.ID != r.lastID {
		return xerrors.New("ended interception ID does not match start")
	}
	return nil
}

func (*countingRecorder) RecordTokenUsage(context.Context, *recorder.TokenUsageRecord) error {
	return nil
}

func (*countingRecorder) RecordPromptUsage(context.Context, *recorder.PromptUsageRecord) error {
	return nil
}

func (*countingRecorder) RecordToolUsage(context.Context, *recorder.ToolUsageRecord) error {
	return nil
}

func (*countingRecorder) RecordModelThought(context.Context, *recorder.ModelThoughtRecord) error {
	return nil
}

func (r *countingRecorder) counts() (starts, ends int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.starts, r.ends
}

func (r *countingRecorder) ended() *recorder.InterceptionRecordEnded {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastEnd == nil {
		return nil
	}
	ended := *r.lastEnd
	return &ended
}

func TestHandlerDoesNotSynthesizeUserAgent(t *testing.T) {
	t.Parallel()

	userAgents := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgents <- r.UserAgent()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "copilot", URL: upstream.URL, Passthrough: []string{"/"}},
		typ:          config.ProviderCopilot, authHeader: "Authorization",
	}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, nil, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/copilot/other", nil))
	require.Equal(t, http.StatusNoContent, response.Code)
	require.Empty(t, <-userAgents)
}

func TestHandlerCentralizedFailoverRecordsFinalKeyHint(t *testing.T) {
	t.Parallel()

	pool, err := keypool.New(config.ProviderOpenAI, []string{
		"first-centralized-provider-key",
		"second-centralized-provider-key",
	}, quartz.NewMock(t), nil)
	require.NoError(t, err)

	var mu sync.Mutex
	var attempts []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		mu.Lock()
		attempts = append(attempts, r.Header.Get("Authorization")+":"+string(body))
		attempt := len(attempts)
		mu.Unlock()
		if attempt == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)

	rec := &testutil.MockRecorder{}
	dumpDir := t.TempDir()
	provider := aibridge.NewOpenAIProvider(config.OpenAI{
		BaseURL:    upstream.URL,
		KeyPool:    pool,
		APIDumpDir: dumpDir,
	})
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString("payload"))
	req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	require.Equal(t, http.StatusNoContent, response.Code)
	mu.Lock()
	require.Equal(t, []string{
		"Bearer first-centralized-provider-key:payload",
		"Bearer second-centralized-provider-key:payload",
	}, attempts)
	mu.Unlock()
	starts := rec.RecordedInterceptions()
	require.Len(t, starts, 1)
	require.Equal(t, recorder.CredentialKindCentralized, starts[0].CredentialKind)
	require.Equal(t, recorder.CredentialHintFailoverKey, starts[0].CredentialHint)
	end := rec.RecordedInterceptionEnd(starts[0].ID)
	require.NotNil(t, end)
	require.Equal(t, "seco...-key", end.CredentialHint)
	require.Empty(t, end.ErrorType)

	dumps, err := filepath.Glob(filepath.Join(dumpDir, config.ProviderOpenAI, "passthrough", "*.req.txt"))
	require.NoError(t, err)
	require.Len(t, dumps, 2)
	for _, dump := range dumps {
		content, err := os.ReadFile(dump)
		require.NoError(t, err)
		require.Contains(t, string(content), "payload")
		require.Contains(t, string(content), "Authorization:")
		require.NotContains(t, string(content), "first-centralized-provider-key")
		require.NotContains(t, string(content), "second-centralized-provider-key")
	}
}

func TestHandlerCircuitOpenStillEndsEveryStartedInterception(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(upstream.Close)
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "openai", URL: upstream.URL, Bridged: []string{"/v1/chat"}},
		typ:          config.ProviderOpenAI, authHeader: "Authorization",
		breaker: &config.CircuitBreaker{FailureThreshold: 1, Timeout: time.Minute},
	}
	rec := &countingRecorder{}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", nil)
		req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		require.Equal(t, http.StatusServiceUnavailable, response.Code)
	}
	starts, ends := rec.counts()
	require.Equal(t, 2, starts)
	require.Equal(t, 2, ends)
}

func TestHandlerStartRecordingBlocksUpstream(t *testing.T) {
	t.Parallel()

	upstreamCalled := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalled <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "openai", URL: upstream.URL, Bridged: []string{"/v1/chat"}},
		typ:          config.ProviderOpenAI, authHeader: "Authorization",
	}
	started := make(chan struct{})
	rec := &countingRecorder{started: started, release: make(chan struct{})}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", nil)
		req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
		router.ServeHTTP(httptest.NewRecorder(), req)
	}()

	<-started
	select {
	case <-upstreamCalled:
		t.Fatal("upstream dispatched before start recording completed")
	default:
	}
	close(rec.release)
	<-done
	select {
	case <-upstreamCalled:
	default:
		t.Fatal("upstream was not dispatched after start recording completed")
	}
	starts, ends := rec.counts()
	require.Equal(t, 1, starts)
	require.Equal(t, 1, ends)
}

func TestHandlerPanicAfterStartEndsExactlyOnceAndPropagates(t *testing.T) {
	t.Parallel()

	configCalls := 0
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "openai", URL: "http://127.0.0.1:1", Bridged: []string{"/v1/chat"}},
		typ:          config.ProviderOpenAI, authHeader: "Authorization",
		failoverCall: func(slog.Logger) keypool.KeyFailoverConfig {
			configCalls++
			return keypool.KeyFailoverConfig{
				Pool: testutil.SingleKeyPool(config.ProviderOpenAI, "panic-key"),
				IsBYOK: func(*http.Request) bool {
					return false
				},
				InjectAuthKey: func(*http.Header, string) {
					panic("after start")
				},
				BuildKeyPoolResponse: func(*keypool.Error) *http.Response { return nil },
			}
		},
	}
	rec := &countingRecorder{}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", nil)
	req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
	require.PanicsWithValue(t, "after start", func() {
		router.ServeHTTP(httptest.NewRecorder(), req)
	})
	require.Equal(t, 1, configCalls)
	starts, ends := rec.counts()
	require.Equal(t, 1, starts)
	require.Equal(t, 1, ends)
}

func TestHandlerForwardsAndRecords(t *testing.T) {
	t.Parallel()

	type upstreamRequest struct {
		path     string
		rawQuery string
		body     string
		header   http.Header
	}
	upstreamRequests := make(chan upstreamRequest, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		upstreamRequests <- upstreamRequest{path: r.URL.EscapedPath(), rawQuery: r.URL.RawQuery, body: string(body), header: r.Header.Clone()}
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("response"))
	}))
	t.Cleanup(upstream.Close)

	const apiKey = "anthropic-personal-api-key" //nolint:gosec // Test provider credential.
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{
			NameStr: "anthropic", URL: upstream.URL + "/base?configured=1",
			Bridged: []string{"/v1/messages"}, Passthrough: []string{"/v1/models"},
		},
		typ: config.ProviderAnthropic, authHeader: "X-Api-Key",
		failover: keypool.KeyFailoverConfig{IsBYOK: func(r *http.Request) bool {
			return r.Header.Get("X-Api-Key") != "" || r.Header.Get("Authorization") != ""
		}},
	}
	rec := &testutil.MockRecorder{}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	payload := `{"metadata":{"user_id":"user_hash_account_id_session_session-123"}}`
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages?raw=a;b", bytes.NewBufferString(payload))
	req = req.WithContext(aibridge.AsActor(req.Context(), "actor-1", recorder.Metadata{"Username": "alice"}))
	req.Header.Set("User-Agent", "claude-cli/1.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("X-Api-Key", apiKey)
	req.Header.Set("Authorization", "Bearer secondary-token")
	req.Header.Set("Coder-Session-Token", "secret")
	req.Header.Set("X-Coder-AI-Governance-Token", "secret")
	req.Header.Set("X-AI-Bridge-Actor-Id", "spoofed")
	req.Header.Set("Forwarded", "for=192.0.2.1")
	req.Header.Set("X-Forwarded-For", "192.0.2.1")
	req.Header.Set(agplaibridge.HeaderAgentFirewallSessionID, "e5f6a7b8-1234-5678-9abc-def012345678")
	req.Header.Set(agplaibridge.HeaderAgentFirewallSequenceNumber, "42")

	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	require.Equal(t, http.StatusCreated, response.Code)
	require.Equal(t, "response", response.Body.String())
	require.Equal(t, "gzip", response.Header().Get("Content-Encoding"))
	gotUpstream := <-upstreamRequests
	require.Equal(t, "/base/v1/messages", gotUpstream.path)
	require.Equal(t, "configured=1&raw=a;b", gotUpstream.rawQuery)
	require.Equal(t, payload, gotUpstream.body)
	require.Equal(t, "claude-cli/1.0", gotUpstream.header.Get("User-Agent"))
	require.Equal(t, "gzip", gotUpstream.header.Get("Accept-Encoding"))
	require.Equal(t, apiKey, gotUpstream.header.Get("X-Api-Key"))
	require.Equal(t, "Bearer secondary-token", gotUpstream.header.Get("Authorization"))
	for name := range gotUpstream.header {
		require.NotRegexp(t, `(?i)^(coder-|x-coder-|x-ai-bridge-actor|forwarded$|x-forwarded-)`, name)
	}

	starts := rec.RecordedInterceptions()
	require.Len(t, starts, 1)
	start := starts[0]
	require.Equal(t, "actor-1", start.InitiatorID)
	require.Equal(t, recorder.Metadata{"Username": "alice"}, start.Metadata)
	require.Equal(t, config.ProviderAnthropic, start.Provider)
	require.Equal(t, "anthropic", start.ProviderName)
	require.Equal(t, "Claude Code", start.Client)
	require.Equal(t, new("session-123"), start.ClientSessionID)
	require.Equal(t, recorder.CredentialKindBYOK, start.CredentialKind)
	require.Equal(t, "anth...-key", start.CredentialHint)
	require.Equal(t, new("e5f6a7b8-1234-5678-9abc-def012345678"), start.AgentFirewallSessionID)
	require.Equal(t, new(int32(42)), start.AgentFirewallSequenceNumber)
	end := rec.RecordedInterceptionEnd(start.ID)
	require.NotNil(t, end)
	require.Empty(t, end.ErrorType)
	require.Empty(t, end.ErrorMessage)
}

func TestHandlerStartFailureDoesNotDispatchOrEnd(t *testing.T) {
	t.Parallel()

	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { upstreamCalled = true }))
	t.Cleanup(upstream.Close)
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "openai", URL: upstream.URL, Bridged: []string{"/v1/chat"}},
		typ:          config.ProviderOpenAI, authHeader: "Authorization",
	}
	rec := &failingStartRecorder{err: xerrors.New("recording unavailable")}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", bytes.NewBufferString("body"))
	req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.False(t, upstreamCalled)
	require.Empty(t, rec.RecordedInterceptions())
}

func TestHandlerClosesIncomingBodyAfterSuccessfulCapture(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, "payload", string(body))
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "openai", URL: upstream.URL, Bridged: []string{"/v1/chat"}},
		typ:          config.ProviderOpenAI, authHeader: "Authorization",
	}
	rec := &testutil.MockRecorder{}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	body := &closeTrackingBody{Reader: strings.NewReader("payload")}
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", body)
	req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	require.Equal(t, http.StatusNoContent, response.Code)
	require.True(t, body.closed)
	rec.VerifyAllInterceptionsEnded(t)
}

type failingResponseWriter struct {
	header http.Header
	status int
	err    error
}

func (w *failingResponseWriter) Header() http.Header { return w.header }
func (w *failingResponseWriter) WriteHeader(status int) {
	w.status = status
}
func (w *failingResponseWriter) Write([]byte) (int, error) { return 0, w.err }

func TestHandlerPoolExhaustionWriteFailureEndsOnceAndAborts(t *testing.T) {
	t.Parallel()

	pool, err := keypool.New(config.ProviderOpenAI, []string{"only-centralized-provider-key"}, quartz.NewMock(t), nil)
	require.NoError(t, err)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(upstream.Close)

	rec := &countingRecorder{}
	provider := aibridge.NewOpenAIProvider(config.OpenAI{BaseURL: upstream.URL, KeyPool: pool})
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(`{}`))
	req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
	writer := &failingResponseWriter{header: http.Header{}, err: xerrors.New("client disconnected")}
	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		router.ServeHTTP(writer, req)
	})

	starts, ends := rec.counts()
	require.Equal(t, 1, starts)
	require.Equal(t, 1, ends)
	ended := rec.ended()
	require.NotNil(t, ended)
	require.Equal(t, recorder.ErrorTypeUnauthorized, ended.ErrorType)
	require.Contains(t, ended.ErrorMessage, "authentication")
}

func TestHandlerPassthroughClientWriteFailureAborts(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response"))
	}))
	t.Cleanup(upstream.Close)
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "copilot", URL: upstream.URL, Passthrough: []string{"/"}},
		typ:          config.ProviderCopilot, authHeader: "Authorization",
	}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, nil, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	req := httptest.NewRequest(http.MethodPost, "/copilot/other", nil)
	writer := &failingResponseWriter{header: http.Header{}, err: xerrors.New("client disconnected")}
	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		router.ServeHTTP(writer, req)
	})
}

func TestHandlerClientWriteFailureEndsOnceAndAborts(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response"))
	}))
	t.Cleanup(upstream.Close)
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "openai", URL: upstream.URL, Bridged: []string{"/v1/chat"}},
		typ:          config.ProviderOpenAI, authHeader: "Authorization",
	}
	rec := &countingRecorder{}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", nil)
	req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
	writer := &failingResponseWriter{header: http.Header{}, err: xerrors.New("client disconnected")}
	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		router.ServeHTTP(writer, req)
	})
	starts, ends := rec.counts()
	require.Equal(t, 1, starts)
	require.Equal(t, 1, ends)
}

func TestHandlerBodyLimitBoundaries(t *testing.T) {
	t.Parallel()

	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Len(t, body, routing.MaxRequestBodyBytes)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "openai", URL: upstream.URL, Bridged: []string{"/v1/chat"}},
		typ:          config.ProviderOpenAI, authHeader: "Authorization",
	}
	rec := &testutil.MockRecorder{}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	exact := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", bytes.NewReader(make([]byte, routing.MaxRequestBodyBytes)))
	exact = exact.WithContext(aibridge.AsActor(exact.Context(), "actor", nil))
	exactResponse := httptest.NewRecorder()
	router.ServeHTTP(exactResponse, exact)
	require.Equal(t, http.StatusNoContent, exactResponse.Code)
	require.Equal(t, 1, upstreamCalls)

	declared := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", bytes.NewBufferString("small"))
	declared = declared.WithContext(aibridge.AsActor(declared.Context(), "actor", nil))
	declared.ContentLength = routing.MaxRequestBodyBytes + 1
	declaredResponse := httptest.NewRecorder()
	router.ServeHTTP(declaredResponse, declared)
	require.Equal(t, http.StatusRequestEntityTooLarge, declaredResponse.Code)
	require.Equal(t, 1, upstreamCalls)
}

func TestHandlerBodyLimitAndCloseOwnership(t *testing.T) {
	t.Parallel()

	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "openai", URL: "http://127.0.0.1:1", Bridged: []string{"/v1/chat"}},
		typ:          config.ProviderOpenAI, authHeader: "Authorization",
	}
	rec := &testutil.MockRecorder{}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	body := &closeTrackingBody{Reader: io.LimitReader(zeroReader{}, routing.MaxRequestBodyBytes+1)}
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", body)
	req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
	req.ContentLength = -1
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	require.True(t, body.closed)
	require.Empty(t, rec.RecordedInterceptions())
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestHandlerConcurrentRequestsKeepRecorderStateSeparate(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "openai", URL: upstream.URL, Bridged: []string{"/v1/chat"}},
		typ:          config.ProviderOpenAI, authHeader: "Authorization",
		failover: keypool.KeyFailoverConfig{IsBYOK: func(r *http.Request) bool { return r.Header.Get("Authorization") != "" }},
	}
	rec := &testutil.MockRecorder{}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	const count = 8
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() {
			actor := "actor-" + string(rune('a'+i))
			req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", bytes.NewBufferString(actor))
			req = req.WithContext(aibridge.AsActor(req.Context(), actor, nil))
			req.Header.Set("Authorization", "Bearer key-"+actor)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			require.Equal(t, http.StatusNoContent, response.Code)
		})
	}
	wg.Wait()

	starts := rec.RecordedInterceptions()
	require.Len(t, starts, count)
	for _, start := range starts {
		require.NotNil(t, rec.RecordedInterceptionEnd(start.ID))
		require.Equal(t, recorder.CredentialKindBYOK, start.CredentialKind)
	}
}
