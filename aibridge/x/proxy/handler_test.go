package proxy_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/coder/v2/aibridge/x/proxy"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	codertestutil "github.com/coder/coder/v2/testutil"
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

type lateTrailerReader struct {
	io.Reader
	trailer http.Header
	values  http.Header
	set     bool
}

func (r *lateTrailerReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF && !r.set {
		for name, values := range r.values {
			r.trailer[name] = append([]string(nil), values...)
		}
		r.set = true
	}
	return n, err
}

func TestHandlerDropsRequestAndResponseTrailers(t *testing.T) {
	t.Parallel()

	const body = "request-body"
	upstreamResult := make(chan error, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, err := io.ReadAll(r.Body)
		if err == nil && string(gotBody) != body {
			err = xerrors.Errorf("unexpected body %q", gotBody)
		}
		if err == nil && len(r.Trailer) != 0 {
			err = xerrors.Errorf("unexpected request trailers: %v", r.Trailer)
		}
		upstreamResult <- err
		w.Header().Add("Trailer", "Set-Cookie")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response-body"))
		w.Header().Set("Set-Cookie", "coder_session_token=upstream-trailer")
	}))
	t.Cleanup(upstream.Close)

	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "copilot", URL: upstream.URL, Passthrough: []string{"/"}},
		typ:          config.ProviderCopilot, authHeader: "Authorization",
	}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, nil, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	gateway := httptest.NewServer(router)
	t.Cleanup(gateway.Close)
	requestTrailers := http.Header{
		"Cookie":               nil,
		"X-AI-Bridge-Actor-Id": nil,
	}
	requestBody := &lateTrailerReader{
		Reader:  strings.NewReader(body),
		trailer: requestTrailers,
		values: http.Header{
			"Cookie":               {"coder_session_token=request-trailer"},
			"X-AI-Bridge-Actor-Id": {"spoofed"},
		},
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, gateway.URL+"/copilot/other", requestBody)
	require.NoError(t, err)
	req.ContentLength = -1
	req.Trailer = requestTrailers
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	gotBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "response-body", string(gotBody))
	require.Empty(t, resp.Trailer)
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	require.NoError(t, codertestutil.RequireReceive(ctx, t, upstreamResult))
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

func TestHandlerCredentialMetadataForAuthorizationSchemes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		authorization string
		wantHint      string
	}{
		{name: "Token", authorization: "Token opaque-provider-token", wantHint: "opaq...oken"},
		{name: "Basic", authorization: "Basic dXNlcjpwYXNz", wantHint: "dX...Nz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(upstream.Close)
			rec := &testutil.MockRecorder{}
			provider, err := aibridge.NewAnthropicProvider(t.Context(), config.Anthropic{BaseURL: upstream.URL}, nil)
			require.NoError(t, err)
			router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, slogtest.Make(t, nil), nil, testTracer(t))
			require.NoError(t, err)
			t.Cleanup(router.CloseIdleConnections)

			req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{}`))
			req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
			req.Header.Set("Authorization", tc.authorization)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)

			require.Equal(t, http.StatusNoContent, response.Code)
			starts := rec.RecordedInterceptions()
			require.Len(t, starts, 1)
			require.Equal(t, recorder.CredentialKindBYOK, starts[0].CredentialKind)
			require.Equal(t, tc.wantHint, starts[0].CredentialHint)
		})
	}
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
		if !assert.NoError(t, err) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
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
	req.Header.Set("Cookie", "coder_session_token=cookie-secret")
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
		require.NotContains(t, string(content), "cookie-secret")
	}
}

func TestHandlerCircuitOpenStillEndsEveryStartedInterception(t *testing.T) {
	t.Parallel()

	upstreamCalls := atomic.Int32{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
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

	responses := make([]*httptest.ResponseRecorder, 0, 2)
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", nil)
		req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		responses = append(responses, response)
		require.Equal(t, http.StatusServiceUnavailable, response.Code)
	}
	require.EqualValues(t, 1, upstreamCalls.Load())
	require.Contains(t, responses[1].Body.String(), "circuit breaker is open")
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

func TestHandlerPassthroughDispatchesBeforeInputEOF(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		_, _ = io.Copy(io.Discard, r.Body)
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

	reader, writer := io.Pipe()
	responseCodes := make(chan int, 1)
	go func() {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/copilot/stream", reader))
		responseCodes <- response.Code
	}()

	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	codertestutil.RequireReceive(ctx, t, started)
	require.NoError(t, writer.Close())
	require.Equal(t, http.StatusNoContent, codertestutil.RequireReceive(ctx, t, responseCodes))
}

func TestHandlerPassthroughUnknownLengthOversizeReturns413(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "copilot", URL: upstream.URL, Passthrough: []string{"/"}},
		typ:          config.ProviderCopilot, authHeader: "Authorization",
	}
	sink := codertestutil.NewFakeSink(t)
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, nil, sink.Logger(), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	body := &closeTrackingBody{Reader: io.LimitReader(zeroReader{}, routing.MaxRequestBodyBytes+1)}
	req := httptest.NewRequest(http.MethodPost, "/copilot/stream", body)
	req.ContentLength = -1
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	require.True(t, body.closed)
	entries := sink.Entries(func(entry slog.SinkEntry) bool { return entry.Message == "rejecting oversized request body" })
	require.Len(t, entries, 1)
	require.Equal(t, slog.LevelWarn, entries[0].Level)
}

func TestHandlerRejectsTraversalAndPreservesEscapedSeparator(t *testing.T) {
	t.Parallel()

	upstreamPaths := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPaths <- r.URL.EscapedPath()
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

	for _, path := range []string{"/copilot/models/%2e%2e/admin", "/copilot/models/%2E/admin"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusBadRequest, response.Code)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/copilot/models/a%2Fb", nil))
	require.Equal(t, http.StatusNoContent, response.Code)
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	require.Equal(t, "/models/a%2Fb", codertestutil.RequireReceive(ctx, t, upstreamPaths))
}

func TestHandlerRequestReadFailureIsSafe(t *testing.T) {
	t.Parallel()

	const secret = "body-secret-value"
	upstreamCalls := atomic.Int32{}
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		upstreamCalls.Add(1)
	}))
	t.Cleanup(upstream.Close)
	sink := codertestutil.NewFakeSink(t)
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "openai", URL: upstream.URL, Bridged: []string{"/v1/chat"}},
		typ:          config.ProviderOpenAI, authHeader: "Authorization",
	}
	rec := &testutil.MockRecorder{}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, sink.Logger(), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", &failingBody{payload: []byte(secret), err: xerrors.New("read failed")})
	req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Zero(t, upstreamCalls.Load())
	require.Empty(t, rec.RecordedInterceptions())
	entries := fmt.Sprint(sink.Entries())
	require.Contains(t, entries, "read failed")
	require.NotContains(t, entries, secret)
}

type failingBody struct {
	payload []byte
	err     error
}

func (b *failingBody) Read(p []byte) (int, error) {
	if len(b.payload) == 0 {
		return 0, b.err
	}
	n := copy(p, b.payload)
	b.payload = b.payload[n:]
	return n, nil
}

func (*failingBody) Close() error { return nil }

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
		if err != nil {
			upstreamRequests <- upstreamRequest{body: "read error: " + err.Error()}
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		upstreamRequests <- upstreamRequest{path: r.URL.EscapedPath(), rawQuery: r.URL.RawQuery, body: string(body), header: r.Header.Clone()}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Set-Cookie", "coder_session_token=upstream")
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
	req.Header.Set("Cookie", "coder_session_token=secret")
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
	require.Empty(t, response.Header().Values("Set-Cookie"))
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	gotUpstream := codertestutil.RequireReceive(ctx, t, upstreamRequests)
	require.Equal(t, "/base/v1/messages", gotUpstream.path)
	require.Equal(t, "configured=1&raw=a;b", gotUpstream.rawQuery)
	require.Equal(t, payload, gotUpstream.body)
	require.Equal(t, "claude-cli/1.0", gotUpstream.header.Get("User-Agent"))
	require.Equal(t, "gzip", gotUpstream.header.Get("Accept-Encoding"))
	require.Equal(t, apiKey, gotUpstream.header.Get("X-Api-Key"))
	require.Equal(t, "Bearer secondary-token", gotUpstream.header.Get("Authorization"))
	for name := range gotUpstream.header {
		require.NotRegexp(t, `(?i)^(cookie$|coder-|x-coder-|x-ai-bridge-actor|forwarded$|x-forwarded-)`, name)
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

	upstreamResult := make(chan error, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err == nil && string(body) != "payload" {
			err = xerrors.Errorf("unexpected body %q", body)
		}
		upstreamResult <- err
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
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
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	require.NoError(t, codertestutil.RequireReceive(ctx, t, upstreamResult))
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
	upstreamResult := make(chan error, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err == nil && len(body) != routing.MaxRequestBodyBytes {
			err = xerrors.Errorf("unexpected body length %d", len(body))
		}
		upstreamResult <- err
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
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
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	require.NoError(t, codertestutil.RequireReceive(ctx, t, upstreamResult))
	upstreamCalls++
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

func TestHandlerPreflightRejections(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		prepare    func(*http.Request) *http.Request
		wantStatus int
		wantBody   string
	}{
		{
			name:       "MissingActor",
			prepare:    func(r *http.Request) *http.Request { return r },
			wantStatus: http.StatusBadRequest,
			wantBody:   "no actor found",
		},
		{
			name: "HTTPUpgradeAnyMethod",
			prepare: func(r *http.Request) *http.Request {
				r = r.WithContext(aibridge.AsActor(r.Context(), "actor", nil))
				r.Method = http.MethodPost
				r.Header.Set("Connection", "Upgrade")
				r.Header.Set("Upgrade", "h2c")
				return r
			},
			wantStatus: http.StatusNotImplemented,
			wantBody:   "HTTP upgrades are not supported",
		},
		{
			name: "MalformedAgentFirewallHeaders",
			prepare: func(r *http.Request) *http.Request {
				r = r.WithContext(aibridge.AsActor(r.Context(), "actor", nil))
				r.Header.Set(agplaibridge.HeaderAgentFirewallSessionID, "not-a-uuid")
				r.Header.Set(agplaibridge.HeaderAgentFirewallSequenceNumber, "1")
				return r
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid agent firewall headers",
		},
		{
			name: "PartialAgentFirewallHeaders",
			prepare: func(r *http.Request) *http.Request {
				r = r.WithContext(aibridge.AsActor(r.Context(), "actor", nil))
				r.Header.Set(agplaibridge.HeaderAgentFirewallSessionID, "e5f6a7b8-1234-5678-9abc-def012345678")
				return r
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid agent firewall headers",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			upstreamCalls := atomic.Int32{}
			upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				upstreamCalls.Add(1)
			}))
			t.Cleanup(upstream.Close)
			rec := &testutil.MockRecorder{}
			provider := &proxyTestProvider{
				MockProvider: &testutil.MockProvider{NameStr: "openai", URL: upstream.URL, Bridged: []string{"/v1/chat"}},
				typ:          config.ProviderOpenAI, authHeader: "Authorization",
			}
			sink := codertestutil.NewFakeSink(t)
			router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, sink.Logger(), nil, testTracer(t))
			require.NoError(t, err)
			t.Cleanup(router.CloseIdleConnections)

			req := tc.prepare(httptest.NewRequest(http.MethodPost, "/openai/v1/chat", nil))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)

			require.Equal(t, tc.wantStatus, response.Code)
			require.Contains(t, response.Body.String(), tc.wantBody)
			require.Zero(t, upstreamCalls.Load())
			require.Empty(t, rec.RecordedInterceptions())
			entries := fmt.Sprint(sink.Entries())
			require.NotEmpty(t, entries)
			require.NotContains(t, entries, "not-a-uuid")
		})
	}
}

func TestHandlerLogsRecorderStartFailureSafely(t *testing.T) {
	t.Parallel()

	const (
		secretHeader = "provider-secret-value"
		secretBody   = "body-secret-value"
	)
	sink := codertestutil.NewFakeSink(t)
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "openai", URL: "http://127.0.0.1:1", Bridged: []string{"/v1/chat"}},
		typ:          config.ProviderOpenAI, authHeader: "Authorization",
	}
	rec := &failingStartRecorder{err: xerrors.New("recording unavailable")}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, sink.Logger(), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", strings.NewReader(secretBody))
	req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
	req.Header.Set("Authorization", "Bearer "+secretHeader)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	require.Equal(t, http.StatusInternalServerError, response.Code)

	entries := fmt.Sprint(sink.Entries())
	require.Contains(t, entries, "failed to record interception")
	require.NotContains(t, entries, secretHeader)
	require.NotContains(t, entries, secretBody)
}

func TestHandlerLogsFailuresSafelyAndTracesUpstream(t *testing.T) {
	t.Parallel()

	const (
		secretHeader = "provider-secret-value"
		secretBody   = "body-secret-value"
	)
	sink := codertestutil.NewFakeSink(t)
	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() { require.NoError(t, tracerProvider.Shutdown(context.Background())) })

	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "copilot", URL: "http://127.0.0.1:1", Passthrough: []string{"/"}},
		typ:          config.ProviderCopilot, authHeader: "Authorization",
	}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, nil, sink.Logger(), nil, tracerProvider.Tracer(t.Name()))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	req := httptest.NewRequest(http.MethodPost, "/copilot/other", strings.NewReader(secretBody))
	req.Header.Set("Authorization", "Bearer "+secretHeader)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	require.Equal(t, http.StatusBadGateway, response.Code)

	entries := sink.Entries()
	require.Contains(t, fmt.Sprint(entries), "reverse proxy error")
	require.NotContains(t, fmt.Sprint(entries), secretHeader)
	require.NotContains(t, fmt.Sprint(entries), secretBody)

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, codes.Error, spans[0].Status().Code)
	require.Contains(t, spans[0].Attributes(), attribute.String(tracing.PassthroughUpstreamURL, "http://127.0.0.1:1/other"))
}

func TestPassthroughMetricCardinalityIsBounded(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	registry := prometheus.NewRegistry()
	m := metrics.NewMetrics(registry)
	provider := aibridge.NewCopilotProvider(config.Copilot{BaseURL: upstream.URL})
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, &testutil.MockRecorder{}, codertestutil.NewFakeSink(t).Logger(), m, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	for i := range 50 {
		response := httptest.NewRecorder()
		path := fmt.Sprintf("/copilot/v1/threads/%d/messages", i)
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusNoContent, response.Code)
	}

	require.Equal(t, 50.0, promtest.ToFloat64(m.PassthroughCount.WithLabelValues(config.ProviderCopilot, "/", http.MethodGet)))
	require.Equal(t, 1, promtest.CollectAndCount(m.PassthroughCount))
}

type failingEndRecorder struct {
	testutil.MockRecorder
	err           error
	endCalls      int
	endContextErr error
}

func (r *failingEndRecorder) RecordInterceptionEnded(ctx context.Context, _ *recorder.InterceptionRecordEnded) error {
	r.endCalls++
	r.endContextErr = ctx.Err()
	return r.err
}

func TestHandlerEndRecordFailureDoesNotMutateTraffic(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	sink := codertestutil.NewFakeSink(t)
	rec := &failingEndRecorder{err: xerrors.New("end recorder unavailable")}
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "openai", URL: upstream.URL, Bridged: []string{"/v1/chat"}},
		typ:          config.ProviderOpenAI, authHeader: "Authorization",
	}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, sink.Logger(), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", nil)
	req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
	req.Header.Set("Authorization", "Bearer provider-secret-value")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	require.Equal(t, http.StatusNoContent, response.Code)
	require.Equal(t, 1, rec.endCalls)
	require.NoError(t, rec.endContextErr)
	entries := fmt.Sprint(sink.Entries())
	require.Contains(t, entries, "failed to record interception end")
	require.NotContains(t, entries, "provider-secret-value")
}

func TestHandlerMetrics(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Failure") != "" {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_, _ = w.Write([]byte("response"))
	}))
	t.Cleanup(upstream.Close)

	registry := prometheus.NewRegistry()
	m := metrics.NewMetrics(registry)
	rec := &testutil.MockRecorder{}
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{
			NameStr: "openai", URL: upstream.URL,
			Bridged: []string{"/v1/chat"}, Passthrough: []string{"/v1/models"},
		},
		typ: config.ProviderOpenAI, authHeader: "Authorization",
	}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, codertestutil.NewFakeSink(t).Logger(), m, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	serve := func(header http.Header, writer http.ResponseWriter) {
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", nil)
		req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
		req.Header = header
		router.ServeHTTP(writer, req)
	}
	serve(http.Header{}, httptest.NewRecorder())
	serve(http.Header{"X-Test-Failure": {"true"}}, httptest.NewRecorder())
	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		serve(http.Header{}, &failingResponseWriter{header: http.Header{}, err: io.ErrClosedPipe})
	})

	passthrough := httptest.NewRecorder()
	router.ServeHTTP(passthrough, httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil))
	require.Equal(t, http.StatusOK, passthrough.Code)
	upstream.Close()
	passthroughFailure := httptest.NewRecorder()
	router.ServeHTTP(passthroughFailure, httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil))
	require.Equal(t, http.StatusBadGateway, passthroughFailure.Code)

	require.Zero(t, promtest.ToFloat64(m.InterceptionsInflight.WithLabelValues("openai", "", "/v1/chat")))
	require.Equal(t, 1.0, promtest.ToFloat64(m.InterceptionCount.WithLabelValues("openai", "", metrics.InterceptionCountStatusCompleted, "/v1/chat", http.MethodPost, "actor", string(aibridge.ClientUnknown))))
	require.Equal(t, 2.0, promtest.ToFloat64(m.InterceptionCount.WithLabelValues("openai", "", metrics.InterceptionCountStatusFailed, "/v1/chat", http.MethodPost, "actor", string(aibridge.ClientUnknown))))
	require.Equal(t, 2.0, promtest.ToFloat64(m.PassthroughCount.WithLabelValues("openai", "/v1/models", http.MethodGet)))

	histogram := promhelp.HistogramValue(t, registry, "interceptions_duration_seconds", prometheus.Labels{
		"provider": "openai",
		"model":    "",
	})
	require.NotNil(t, histogram)
	require.EqualValues(t, 3, histogram.GetSampleCount())
}

func TestHandlerProviderStatusClassificationAndFailureLog(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		status      int
		newProvider func(*testing.T, string) aibridge.Provider
		path        string
		wantMsg     string
	}{
		{
			name:   "Anthropic529",
			status: 529,
			newProvider: func(t *testing.T, baseURL string) aibridge.Provider {
				provider, err := aibridge.NewAnthropicProvider(t.Context(), config.Anthropic{BaseURL: baseURL}, nil)
				require.NoError(t, err)
				return provider
			},
			path:    "/anthropic/v1/messages",
			wantMsg: "HTTP status 529",
		},
		{
			name:   "OpenAI503",
			status: http.StatusServiceUnavailable,
			newProvider: func(_ *testing.T, baseURL string) aibridge.Provider {
				return aibridge.NewOpenAIProvider(config.OpenAI{BaseURL: baseURL})
			},
			path:    "/openai/v1/chat/completions",
			wantMsg: http.StatusText(http.StatusServiceUnavailable),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(upstream.Close)
			rec := &testutil.MockRecorder{}
			sink := codertestutil.NewFakeSink(t)
			router, err := proxy.NewRouter([]aibridge.Provider{tc.newProvider(t, upstream.URL)}, rec, sink.Logger(), nil, testTracer(t))
			require.NoError(t, err)
			t.Cleanup(router.CloseIdleConnections)

			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(`{}`))
			req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			require.Equal(t, tc.status, response.Code)

			starts := rec.RecordedInterceptions()
			require.Len(t, starts, 1)
			end := rec.RecordedInterceptionEnd(starts[0].ID)
			require.NotNil(t, end)
			require.Equal(t, recorder.ErrorTypeOverloaded, end.ErrorType)
			require.Equal(t, tc.wantMsg, end.ErrorMessage)
			entries := sink.Entries(func(entry slog.SinkEntry) bool { return entry.Message == "interception failed" })
			require.Len(t, entries, 1)
			require.Equal(t, slog.LevelWarn, entries[0].Level)
			require.Contains(t, fmt.Sprint(entries[0].Fields), starts[0].ID)
			require.Contains(t, fmt.Sprint(entries[0].Fields), string(recorder.ErrorTypeOverloaded))
		})
	}
}

func TestHandlerRecordsSharedErrorClassifications(t *testing.T) {
	t.Parallel()

	t.Run("HTTPRateLimit", func(t *testing.T) {
		t.Parallel()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		t.Cleanup(upstream.Close)
		rec := &testutil.MockRecorder{}
		provider := &proxyTestProvider{
			MockProvider: &testutil.MockProvider{NameStr: "copilot", URL: upstream.URL, Bridged: []string{"/v1/chat"}},
			typ:          config.ProviderCopilot, authHeader: "Authorization",
		}
		router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, codertestutil.NewFakeSink(t).Logger(), nil, testTracer(t))
		require.NoError(t, err)
		t.Cleanup(router.CloseIdleConnections)
		req := httptest.NewRequest(http.MethodPost, "/copilot/v1/chat", nil)
		req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
		req.Header.Set("Authorization", "Bearer token")
		router.ServeHTTP(httptest.NewRecorder(), req)
		starts := rec.RecordedInterceptions()
		require.Len(t, starts, 1)
		require.Equal(t, recorder.ErrorTypeRateLimited, rec.RecordedInterceptionEnd(starts[0].ID).ErrorType)
	})

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()
		rec := &testutil.MockRecorder{}
		sink := codertestutil.NewFakeSink(t)
		provider := &proxyTestProvider{
			MockProvider: &testutil.MockProvider{NameStr: "openai", URL: "http://127.0.0.1:1", Bridged: []string{"/v1/chat"}},
			typ:          config.ProviderOpenAI, authHeader: "Authorization",
		}
		router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, sink.Logger(), nil, testTracer(t))
		require.NoError(t, err)
		t.Cleanup(router.CloseIdleConnections)
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		defer cancel()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", bytes.NewReader(nil)).WithContext(ctx)
		req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
		require.PanicsWithValue(t, http.ErrAbortHandler, func() {
			router.ServeHTTP(httptest.NewRecorder(), req)
		})
		starts := rec.RecordedInterceptions()
		require.Len(t, starts, 1)
		require.Equal(t, recorder.ErrorTypeTimeout, rec.RecordedInterceptionEnd(starts[0].ID).ErrorType)
		entries := sink.Entries(func(entry slog.SinkEntry) bool { return entry.Message == "interception failed" })
		require.Len(t, entries, 1)
		require.Equal(t, slog.LevelDebug, entries[0].Level)
	})
}
