package integrationtest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/credential"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/coder/v2/aibridge/x/proxy/responsesws"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	codertestutil "github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

const (
	wsCreate    = `{"type":"response.create","model":"gpt-5","input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}]}`
	wsCreated   = `{"type":"response.created","response":{"id":"resp_1","model":"gpt-5","status":"in_progress"}}`
	wsDelta     = `{"type":"response.output_text.delta","delta":"hi"}`
	wsCompleted = `{"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":30},"output_tokens":7,"output_tokens_details":{"reasoning_tokens":3},"total_tokens":107}}}`
)

// wsUpstream is a fake OpenAI Responses WebSocket server. It records the
// headers of every handshake and hands each accepted connection to the test.
type wsUpstream struct {
	*httptest.Server
	conns chan *websocket.Conn

	mu         sync.Mutex
	handshakes []http.Header
}

// newWSUpstream starts a fake upstream. reject, when set, refuses a
// handshake with the status it returns, or accepts it on 0.
func newWSUpstream(t *testing.T, reject func(*http.Request) int) *wsUpstream {
	t.Helper()
	u := &wsUpstream{conns: make(chan *websocket.Conn, 8)}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		u.handshakes = append(u.handshakes, r.Header.Clone())
		u.mu.Unlock()
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}
		if reject != nil {
			if status := reject(r); status != 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = fmt.Fprintf(w, `{"error":{"message":"handshake refused","type":"invalid_request_error","code":"refused_%d"}}`, status)
				return
			}
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("upstream accept: %v", err)
			return
		}
		conn.SetReadLimit(-1)
		u.conns <- conn
	}))
	t.Cleanup(u.Close)
	return u
}

func (u *wsUpstream) accept(ctx context.Context, t *testing.T) *websocket.Conn {
	t.Helper()
	conn := codertestutil.TryReceive(ctx, t, u.conns)
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func (u *wsUpstream) handshakeHeaders() []http.Header {
	u.mu.Lock()
	defer u.mu.Unlock()
	return slices.Clone(u.handshakes)
}

// testLease mirrors aibridged's lease: its context keeps the request's
// values but not its cancellation.
type testLease struct {
	ctx      context.Context
	cancel   context.CancelCauseFunc
	released chan struct{}
	once     sync.Once
}

func (l *testLease) Context() context.Context { return l.ctx }

func (l *testLease) Release() {
	l.once.Do(func() {
		l.cancel(nil)
		close(l.released)
	})
}

type testAcquirer struct {
	leases chan *testLease
}

func (a *testAcquirer) acquire(ctx context.Context, _, _ string) (aibcontext.SocketLease, error) {
	leaseCtx, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
	lease := &testLease{ctx: leaseCtx, cancel: cancel, released: make(chan struct{})}
	a.leases <- lease
	return lease, nil
}

type wsGatewayConfig struct {
	// disabled turns the experiment off for the user.
	disabled   bool
	noAcquirer bool
	admit      aibcontext.CreateAdmissionFunc
	reject     func(*http.Request) int
	// provider, when set, is the only provider, built for the upstream URL.
	provider func(upstreamURL string) aibridge.Provider
	opts     []bridgeOption
}

// wsGateway is a bridge with a fake upstream, set up like aibridged sets up
// an authorized request.
type wsGateway struct {
	*bridgeTestServer
	upstream *wsUpstream
	leases   *testAcquirer
}

func newWSGateway(ctx context.Context, t *testing.T, cfg wsGatewayConfig) *wsGateway {
	t.Helper()
	upstream := newWSUpstream(t, cfg.reject)
	leases := &testAcquirer{leases: make(chan *testLease, 8)}
	opts := append([]bridgeOption{withRequestContext(func(ctx context.Context) context.Context {
		ctx = agplaibridge.WithResponsesWebSocketEnabled(ctx, !cfg.disabled)
		if !cfg.noAcquirer {
			ctx = aibcontext.WithSocketAcquirer(ctx, leases.acquire)
		}
		if cfg.admit != nil {
			ctx = aibcontext.WithCreateAdmission(ctx, cfg.admit)
		}
		return ctx
	})}, cfg.opts...)
	if cfg.provider != nil {
		opts = append(opts, func(c *bridgeConfig) {
			c.providerBuilders = append(c.providerBuilders, func(_ *testing.T, upstreamURL string) aibridge.Provider {
				return cfg.provider(upstreamURL)
			})
		})
	}
	return &wsGateway{
		bridgeTestServer: newBridgeTestServer(ctx, t, upstream.URL, opts...),
		upstream:         upstream,
		leases:           leases,
	}
}

func (g *wsGateway) dial(ctx context.Context, path string, header http.Header) (*websocket.Conn, *http.Response, error) {
	//nolint:bodyclose // Dial owns the response body.
	return websocket.Dial(ctx, "ws"+strings.TrimPrefix(g.URL, "http")+path, &websocket.DialOptions{HTTPHeader: header})
}

// open dials the Responses route and returns the client connection, the
// upstream connection, and the socket's lease.
func (g *wsGateway) open(ctx context.Context, t *testing.T, header http.Header) (client, upstream *websocket.Conn, lease *testLease) {
	t.Helper()
	//nolint:bodyclose // Dial owns the response body.
	client, resp, err := g.dial(ctx, pathOpenAIResponses, header)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	lease = codertestutil.TryReceive(ctx, t, g.leases.leases)
	awaitRelease(t, lease)
	t.Cleanup(func() { _ = client.CloseNow() })
	return client, g.upstream.accept(ctx, t), lease
}

// awaitRelease makes the test wait, after its connections closed, until
// the socket finished tearing down. Register it before the cleanups that
// close the connections.
func awaitRelease(t *testing.T, lease *testLease) {
	t.Helper()
	// Created first, so its cancel runs after the wait.
	ctx := codertestutil.Context(t, codertestutil.WaitLong)
	t.Cleanup(func() {
		select {
		case <-lease.released:
		case <-ctx.Done():
			t.Error("socket was not released")
		}
	})
}

func writeText(ctx context.Context, t *testing.T, conn *websocket.Conn, msg string) {
	t.Helper()
	require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(msg)))
}

func readText(ctx context.Context, t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	typ, msg, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageText, typ)
	return string(msg)
}

// requireClosed reads conn until its peer closes it and returns the close.
func requireClosed(ctx context.Context, t *testing.T, conn *websocket.Conn, code websocket.StatusCode) websocket.CloseError {
	t.Helper()
	for {
		_, _, err := conn.Read(ctx)
		if err == nil {
			continue
		}
		closeErr, ok := errors.AsType[websocket.CloseError](err)
		require.True(t, ok, "expected a close frame, got %v", err)
		require.Equal(t, code, closeErr.Code, "close reason %q", closeErr.Reason)
		return closeErr
	}
}

// TestResponsesWebSocketUpgradeGate requires that only the OpenAI Responses
// route serves WebSocket mode, only for users with the experiment on, and
// only when the gateway supplied socket capacity; every other upgrade is
// refused before anything is dialed.
func TestResponsesWebSocketUpgradeGate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		cfg        wsGatewayConfig
		path       string
		copilot    bool
		wantStatus int
	}{
		{name: "Enabled", path: pathOpenAIResponses, wantStatus: http.StatusSwitchingProtocols},
		{name: "ExperimentOff", cfg: wsGatewayConfig{disabled: true}, path: pathOpenAIResponses, wantStatus: http.StatusNotImplemented},
		{name: "NoSocketAcquirer", cfg: wsGatewayConfig{noAcquirer: true}, path: pathOpenAIResponses, wantStatus: http.StatusNotImplemented},
		{name: "ChatCompletions", path: pathOpenAIChatCompletions, wantStatus: http.StatusNotImplemented},
		{name: "CopilotResponses", path: pathCopilotResponses, copilot: true, wantStatus: http.StatusNotImplemented},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := codertestutil.Context(t, codertestutil.WaitLong)
			cfg := tc.cfg
			if tc.copilot {
				cfg.provider = func(upstreamURL string) aibridge.Provider {
					return provider.NewCopilot(config.Copilot{BaseURL: upstreamURL})
				}
			}
			g := newWSGateway(ctx, t, cfg)

			if tc.wantStatus == http.StatusSwitchingProtocols {
				_, _, _ = g.open(ctx, t, nil)
				return
			}
			//nolint:bodyclose // Dial owns the response body.
			_, resp, err := g.dial(ctx, tc.path, nil)
			require.Error(t, err)
			require.NotNil(t, resp)
			require.Equal(t, tc.wantStatus, resp.StatusCode)
			assert.Empty(t, g.upstream.handshakeHeaders())
			assert.Empty(t, g.leases.leases)
		})
	}
}

// TestResponsesWebSocketRelaysAndRecords requires that every frame crosses
// the gateway unchanged in both directions, MCP tools included for the user
// or not, and that the create is recorded with its prompt, token usage, and
// end.
func TestResponsesWebSocketRelaysAndRecords(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitLong)
	// The user has MCP tools: a WebSocket session must not inject them.
	g := newWSGateway(ctx, t, wsGatewayConfig{opts: []bridgeOption{withMCP(setupMCPForTest(t, defaultTracer))}})
	client, upstream, lease := g.open(ctx, t, http.Header{"User-Agent": {"codex_cli_rs/0.50.0"}})

	for _, frame := range []string{wsCreate, `{"type":"response.inject","response_id":"resp_0","input":"more"}`} {
		writeText(ctx, t, client, frame)
		require.Equal(t, frame, readText(ctx, t, upstream))
	}
	for _, frame := range []string{wsCreated, wsDelta, `{"type":"response.custom", "x":[1, 2]}`, wsCompleted} {
		writeText(ctx, t, upstream, frame)
		require.Equal(t, frame, readText(ctx, t, client))
	}

	require.NoError(t, client.Close(websocket.StatusNormalClosure, ""))
	requireClosed(ctx, t, upstream, websocket.StatusNormalClosure)
	_ = codertestutil.TryReceive(ctx, t, lease.released)

	interceptions := g.Recorder.RecordedInterceptions()
	require.Len(t, interceptions, 1)
	ic := interceptions[0]
	assert.Equal(t, defaultActorID, ic.InitiatorID)
	assert.Equal(t, "gpt-5", ic.Model)
	assert.Equal(t, config.ProviderOpenAI, ic.ProviderName)
	assert.Equal(t, "codex_cli_rs/0.50.0", ic.UserAgent)
	assert.Equal(t, credential.KindCentralized, ic.CredentialKind)
	assert.Equal(t, utils.MaskSecret(apiKey), ic.CredentialHint)
	prompts := g.Recorder.RecordedPromptUsages()
	require.Len(t, prompts, 1)
	assert.Equal(t, "hello", prompts[0].Prompt)
	tokens := g.Recorder.RecordedTokenUsages()
	require.Len(t, tokens, 1)
	assert.Equal(t, ic.ID, tokens[0].InterceptionID)
	assert.EqualValues(t, 7, tokens[0].Output)
	require.NotNil(t, g.Recorder.RecordedInterceptionEnd(ic.ID))
}

// TestResponsesWebSocketAgentFirewallHeaders requires that a socket opened
// through Agent Firewall records its correlation headers on its
// interceptions, and that partial or malformed headers are refused before
// a lease is taken or anything is dialed, as an HTTP request is.
func TestResponsesWebSocketAgentFirewallHeaders(t *testing.T) {
	t.Parallel()
	const sessionID = "6f4c7c1a-1b4e-4f4a-9a43-2f6b8e6f7d10"

	t.Run("Recorded", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitLong)
		g := newWSGateway(ctx, t, wsGatewayConfig{})
		client, upstream, lease := g.open(ctx, t, http.Header{
			agplaibridge.HeaderAgentFirewallSessionID:      {sessionID},
			agplaibridge.HeaderAgentFirewallSequenceNumber: {"42"},
		})
		writeText(ctx, t, client, wsCreate)
		require.Equal(t, wsCreate, readText(ctx, t, upstream))
		require.NoError(t, client.Close(websocket.StatusNormalClosure, ""))
		requireClosed(ctx, t, upstream, websocket.StatusNormalClosure)
		_ = codertestutil.TryReceive(ctx, t, lease.released)

		interceptions := g.Recorder.RecordedInterceptions()
		require.Len(t, interceptions, 1)
		assert.Equal(t, ptr.Ref(sessionID), interceptions[0].AgentFirewallSessionID)
		assert.Equal(t, ptr.Ref(int32(42)), interceptions[0].AgentFirewallSequenceNumber)
		for _, h := range g.upstream.handshakeHeaders() {
			assert.Empty(t, h.Get(agplaibridge.HeaderAgentFirewallSessionID))
			assert.Empty(t, h.Get(agplaibridge.HeaderAgentFirewallSequenceNumber))
		}
	})
	t.Run("Malformed", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitLong)
		g := newWSGateway(ctx, t, wsGatewayConfig{})
		//nolint:bodyclose // Dial owns the response body.
		_, resp, err := g.dial(ctx, pathOpenAIResponses, http.Header{
			agplaibridge.HeaderAgentFirewallSessionID: {sessionID},
		})
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Empty(t, g.leases.leases, "a refused socket must not take a lease")
		assert.Empty(t, g.upstream.handshakeHeaders(), "a refused socket must not dial upstream")
	})
}

// recordingTransport records the headers of every request it sends.
type recordingTransport struct {
	mu      sync.Mutex
	headers []http.Header
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.headers = append(rt.headers, req.Header.Clone())
	rt.mu.Unlock()
	return http.DefaultTransport.RoundTrip(req)
}

// TestResponsesWebSocketUpstreamHeaders requires that upstream sees only
// the resolved credential: the centralized key, or the user's own key, and
// never a Coder credential, cookie, or the client's handshake key, while
// other client headers pass through.
func TestResponsesWebSocketUpstreamHeaders(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		auth     string
		wantAuth string
	}{
		{name: "Centralized", wantAuth: "Bearer " + apiKey},
		{name: "BYOK", auth: "Bearer user-key", wantAuth: "Bearer user-key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := codertestutil.Context(t, codertestutil.WaitLong)
			g := newWSGateway(ctx, t, wsGatewayConfig{})
			header := http.Header{
				"X-Client-Header":             {"kept"},
				"Cookie":                      {"coder_session_token=secret"},
				codersdk.SessionTokenHeader:   {"coder-secret"},
				agplaibridge.HeaderCoderToken: {"coder-secret"},
				// The gateway negotiates its own handshake with upstream,
				// and a browser's Origin is not forwarded.
				"Sec-WebSocket-Protocol":   {"client-protocol"},
				"Sec-WebSocket-Extensions": {"permessage-deflate"},
				"Origin":                   {g.URL},
			}
			if tc.auth != "" {
				header.Set("Authorization", tc.auth)
			}
			sent := &recordingTransport{}
			//nolint:bodyclose // Dial owns the response body.
			client, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(g.URL, "http")+pathOpenAIResponses, &websocket.DialOptions{
				HTTPClient: &http.Client{Transport: sent},
				HTTPHeader: header,
			})
			require.NoError(t, err)
			awaitRelease(t, codertestutil.TryReceive(ctx, t, g.leases.leases))
			t.Cleanup(func() { _ = client.CloseNow() })
			_ = g.upstream.accept(ctx, t)

			handshakes := g.upstream.handshakeHeaders()
			require.Len(t, handshakes, 1)
			got := handshakes[0]
			assert.Equal(t, []string{tc.wantAuth}, got.Values("Authorization"))
			assert.Equal(t, "kept", got.Get("X-Client-Header"))
			for _, name := range []string{"Cookie", codersdk.SessionTokenHeader, agplaibridge.HeaderCoderToken, "Sec-WebSocket-Protocol", "Sec-WebSocket-Extensions", "Origin"} {
				assert.Empty(t, got.Values(name), name)
			}
			require.Len(t, sent.headers, 1)
			assert.NotEqual(t, sent.headers[0].Get("Sec-WebSocket-Key"), got.Get("Sec-WebSocket-Key"))
		})
	}
}

// TestResponsesWebSocketKeyFailover requires that a centralized pool fails
// over to its next key when upstream refuses a handshake as HTTP requests
// do, that the socket is recorded with the key it used, and that a socket
// whose upstream closed after the handshake is never reconnected: the
// client receives the upstream close status instead.
func TestResponsesWebSocketKeyFailover(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			ctx := codertestutil.Context(t, codertestutil.WaitLong)
			pool, err := keypool.New(config.ProviderOpenAI, []string{"key-a", "key-b"}, quartz.NewReal(), nil)
			require.NoError(t, err)
			g := newWSGateway(ctx, t, wsGatewayConfig{
				reject: func(r *http.Request) int {
					if r.Header.Get("Authorization") == "Bearer key-a" {
						return status
					}
					return 0
				},
				provider: func(upstreamURL string) aibridge.Provider {
					return provider.NewOpenAI(config.OpenAI{BaseURL: upstreamURL, KeyPool: pool, ResponsesWebSocket: true})
				},
			})
			client, upstream, lease := g.open(ctx, t, nil)

			writeText(ctx, t, client, wsCreate)
			require.Equal(t, wsCreate, readText(ctx, t, upstream))
			require.NoError(t, upstream.Close(4000, "upstream says bye"))
			closeErr := requireClosed(ctx, t, client, 4000)
			assert.Equal(t, "upstream says bye", closeErr.Reason)
			_ = codertestutil.TryReceive(ctx, t, lease.released)

			var auths []string
			for _, h := range g.upstream.handshakeHeaders() {
				auths = append(auths, h.Get("Authorization"))
			}
			assert.Equal(t, []string{"Bearer key-a", "Bearer key-b"}, auths)
			interceptions := g.Recorder.RecordedInterceptions()
			require.Len(t, interceptions, 1)
			assert.Equal(t, utils.MaskSecret("key-b"), interceptions[0].CredentialHint)
		})
	}
}

// TestResponsesWebSocketClientLeavesDuringHandshake requires that a client
// leaving before its upgrade ends the upstream handshake and releases the
// socket at once, instead of holding the lease for the handshake timeout.
func TestResponsesWebSocketClientLeavesDuringHandshake(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	entered := make(chan struct{}, 1)
	g := newWSGateway(ctx, t, wsGatewayConfig{reject: func(r *http.Request) int {
		// The upstream handshake hangs until the gateway abandons it.
		entered <- struct{}{}
		<-r.Context().Done()
		return http.StatusServiceUnavailable
	}})

	dialCtx, cancel := context.WithCancel(ctx)
	dialed := make(chan error, 1)
	go func() {
		//nolint:bodyclose // Dial owns the response body.
		_, _, err := g.dial(dialCtx, pathOpenAIResponses, nil)
		dialed <- err
	}()
	lease := codertestutil.TryReceive(ctx, t, g.leases.leases)
	_ = codertestutil.TryReceive(ctx, t, entered)
	cancel()
	require.Error(t, codertestutil.TryReceive(ctx, t, dialed))
	_ = codertestutil.TryReceive(ctx, t, lease.released)
}

// TestResponsesWebSocketHandshakeFailure requires that a failed upstream
// handshake refuses the client with a plain HTTP error, as the HTTP route
// would, so the client can fall back to HTTP, and that the lease is freed.
func TestResponsesWebSocketHandshakeFailure(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		auth           string
		keys           []string
		status         int
		closedUpstream bool
		wantStatus     int
		wantHandshakes int
	}{
		// A user's own key never fails over.
		{name: "BYOKUnauthorized", auth: "Bearer user-key", status: http.StatusUnauthorized, wantStatus: http.StatusUnauthorized, wantHandshakes: 1},
		// Only key specific statuses fail over.
		{name: "ServerError", keys: []string{"key-a", "key-b"}, status: http.StatusInternalServerError, wantStatus: http.StatusInternalServerError, wantHandshakes: 1},
		{name: "PoolExhausted", keys: []string{"key-a", "key-b"}, status: http.StatusUnauthorized, wantStatus: http.StatusBadGateway, wantHandshakes: 2},
		{name: "NetworkError", keys: []string{"key-a"}, closedUpstream: true, wantStatus: http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := codertestutil.Context(t, codertestutil.WaitLong)
			cfg := wsGatewayConfig{reject: func(*http.Request) int { return tc.status }}
			if tc.keys != nil {
				pool, err := keypool.New(config.ProviderOpenAI, tc.keys, quartz.NewReal(), nil)
				require.NoError(t, err)
				cfg.provider = func(upstreamURL string) aibridge.Provider {
					return provider.NewOpenAI(config.OpenAI{BaseURL: upstreamURL, KeyPool: pool, ResponsesWebSocket: true})
				}
			}
			g := newWSGateway(ctx, t, cfg)
			if tc.closedUpstream {
				g.upstream.Close()
			}
			header := http.Header{}
			if tc.auth != "" {
				header.Set("Authorization", tc.auth)
			}

			//nolint:bodyclose // Dial owns the response body.
			_, resp, err := g.dial(ctx, pathOpenAIResponses, header)
			require.Error(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, tc.wantStatus, resp.StatusCode)
			if !tc.closedUpstream {
				assert.Len(t, g.upstream.handshakeHeaders(), tc.wantHandshakes)
			}
			lease := codertestutil.TryReceive(ctx, t, g.leases.leases)
			_ = codertestutil.TryReceive(ctx, t, lease.released)
			assert.Empty(t, g.Recorder.RecordedInterceptions())
		})
	}
}

// TestResponsesWebSocketInvalidClientHandshake requires that an upgrade the
// gateway cannot accept is refused before it takes a lease or opens an
// upstream connection, which would use up a key attempt.
func TestResponsesWebSocketInvalidClientHandshake(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitLong)
	g := newWSGateway(ctx, t, wsGatewayConfig{})

	// A cross-origin browser page may not open the socket.
	//nolint:bodyclose // Dial owns the response body.
	_, resp, err := g.dial(ctx, pathOpenAIResponses, http.Header{"Origin": {"https://evil.example.com"}})
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Empty(t, g.upstream.handshakeHeaders())
	assert.Empty(t, g.leases.leases)
}

// TestResponsesWebSocketCloseMapping requires that each way a socket ends
// closes both sides with a status that tells them why, and frees the lease.
func TestResponsesWebSocketCloseMapping(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// end ends the socket.
		end          func(ctx context.Context, t *testing.T, client, upstream *websocket.Conn, lease *testLease)
		wantClient   websocket.StatusCode
		wantUpstream websocket.StatusCode
		wantReason   string
	}{
		{
			name: "ClientClose",
			end: func(ctx context.Context, t *testing.T, client, _ *websocket.Conn, _ *testLease) {
				go func() { _ = client.Close(4001, "client done") }()
			},
			wantUpstream: 4001,
			wantReason:   "client done",
		},
		{
			name: "ClientBinaryMessage",
			end: func(ctx context.Context, t *testing.T, client, _ *websocket.Conn, _ *testLease) {
				require.NoError(t, client.Write(ctx, websocket.MessageBinary, []byte("{}")))
			},
			wantClient:   websocket.StatusUnsupportedData,
			wantUpstream: websocket.StatusGoingAway,
		},
		{
			name: "ClientMessageTooBig",
			end: func(ctx context.Context, t *testing.T, client, _ *websocket.Conn, _ *testLease) {
				// The write may fail once the gateway closed the connection.
				_ = client.Write(ctx, websocket.MessageText, make([]byte, responsesws.MaxClientFrameBytes+1))
			},
			wantClient:   websocket.StatusMessageTooBig,
			wantUpstream: websocket.StatusGoingAway,
		},
		{
			name: "UpstreamMessageTooBig",
			end: func(ctx context.Context, t *testing.T, _, upstream *websocket.Conn, _ *testLease) {
				_ = upstream.Write(ctx, websocket.MessageText, make([]byte, responsesws.MaxUpstreamFrameBytes+1))
			},
			wantClient:   websocket.StatusMessageTooBig,
			wantUpstream: websocket.StatusMessageTooBig,
		},
		{
			// The gateway ended the socket, at its maximum lifetime or on
			// shutdown.
			name: "LeaseEnded",
			end: func(_ context.Context, _ *testing.T, _, _ *websocket.Conn, lease *testLease) {
				lease.cancel(xerrors.New("socket reached its maximum lifetime"))
			},
			wantClient:   websocket.StatusGoingAway,
			wantUpstream: websocket.StatusGoingAway,
			wantReason:   "socket reached its maximum lifetime",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := codertestutil.Context(t, codertestutil.WaitLong)
			g := newWSGateway(ctx, t, wsGatewayConfig{})
			client, upstream, lease := g.open(ctx, t, nil)
			client.SetReadLimit(-1)

			// An open interception ends with the socket.
			writeText(ctx, t, client, wsCreate)
			require.Equal(t, wsCreate, readText(ctx, t, upstream))

			tc.end(ctx, t, client, upstream, lease)
			if tc.wantClient != 0 {
				closeErr := requireClosed(ctx, t, client, tc.wantClient)
				if tc.wantReason != "" {
					assert.Equal(t, tc.wantReason, closeErr.Reason)
				}
			}
			closeErr := requireClosed(ctx, t, upstream, tc.wantUpstream)
			if tc.wantReason != "" {
				assert.Equal(t, tc.wantReason, closeErr.Reason)
			}
			_ = codertestutil.TryReceive(ctx, t, lease.released)
			interceptions := g.Recorder.RecordedInterceptions()
			require.Len(t, interceptions, 1)
			require.NotNil(t, g.Recorder.RecordedInterceptionEnd(interceptions[0].ID))
		})
	}
}

// TestResponsesWebSocketCreateAdmission requires that each create runs the
// gateway's admission: a refused create is answered with an error event and
// never reaches upstream, and the socket stays open for the next create.
func TestResponsesWebSocketCreateAdmission(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitLong)
	var mu sync.Mutex
	refuse := true
	g := newWSGateway(ctx, t, wsGatewayConfig{admit: func(context.Context, string) error {
		mu.Lock()
		defer mu.Unlock()
		if refuse {
			refuse = false
			return intercept.NewResponseError("rate limited", intercept.OpenAIErrTypeRateLimit, intercept.OpenAIErrCodeRateLimit, http.StatusTooManyRequests, 0)
		}
		return nil
	}})
	client, upstream, _ := g.open(ctx, t, nil)

	writeText(ctx, t, client, wsCreate)
	ev := readText(ctx, t, client)
	assert.Equal(t, "error", gjson.Get(ev, "type").String())
	assert.EqualValues(t, http.StatusTooManyRequests, gjson.Get(ev, "status").Int())
	assert.Equal(t, intercept.OpenAIErrCodeRateLimit, gjson.Get(ev, "error.code").String())

	second := strings.Replace(wsCreate, "hello", "again", 1)
	writeText(ctx, t, client, second)
	require.Equal(t, second, readText(ctx, t, upstream))
}
