//go:build !slim

package cli

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// testAIChatCompletionRequest is the LLM request the reconnection tests send.
const testAIChatCompletionRequest = `{"messages":[{"role":"user","content":"standalone gateway test"}],"model":"gpt-4.1"}`

// testAIStreamingChatCompletionRequest asks for a streamed completion so the
// upstream can write response bytes before the test drops the DRPC connection.
const testAIStreamingChatCompletionRequest = `{"messages":[{"role":"user","content":"standalone gateway test"}],"model":"gpt-4.1","stream":true}`

const (
	mockAIStreamFirstChunk = `data: {"id":"chatcmpl-standalone-test","object":"chat.completion.chunk","created":1753343279,"model":"gpt-4.1","choices":[{"index":0,"delta":{"role":"assistant","content":"first half"},"finish_reason":null}]}` + "\n\n"
	mockAIStreamFinalChunk = `data: {"id":"chatcmpl-standalone-test","object":"chat.completion.chunk","created":1753343279,"model":"gpt-4.1","choices":[{"index":0,"delta":{"content":" second half"},"finish_reason":"stop"}]}` + "\n\n" + "data: [DONE]\n\n"
)

// severableListener records the connections it accepts so a test can sever
// them. It is needed because net/http stops tracking a connection once a
// handler hijacks it for a protocol switch, which puts the gateway's websocket
// out of reach of httptest.Server.CloseClientConnections.
type severableListener struct {
	net.Listener

	mu    sync.Mutex
	conns []net.Conn
}

func (l *severableListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.conns = append(l.conns, conn)
	return conn, nil
}

// sever closes every connection accepted so far and stops tracking them, so
// connections established afterwards survive.
func (l *severableListener) sever() {
	l.mu.Lock()
	conns := l.conns
	l.conns = nil
	l.mu.Unlock()

	for _, conn := range conns {
		_ = conn.Close()
	}
}

// chaosProxy sits between the standalone gateway and coderd so tests can
// simulate network failures and coderd unavailability without stopping
// coderdenttest. While unhealthy it answers 503, which the connect loop
// classifies as transient and retries.
type chaosProxy struct {
	srv      *httptest.Server
	listener *severableListener
	healthy  atomic.Bool
}

func newChaosProxy(t *testing.T, target *url.URL) *chaosProxy {
	t.Helper()

	p := &chaosProxy{}
	p.healthy.Store(true)
	reverse := httputil.NewSingleHostReverseProxy(target)
	reverse.ErrorLog = log.New(testutil.NewTestLogWriter(t), "chaosProxy: ", 0)
	p.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !p.healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		reverse.ServeHTTP(w, r)
	}))
	p.listener = &severableListener{Listener: p.srv.Listener}
	p.srv.Listener = p.listener
	p.srv.Start()
	t.Cleanup(p.srv.Close)
	return p
}

func (p *chaosProxy) URL(t *testing.T) *url.URL {
	t.Helper()

	u, err := url.Parse(p.srv.URL)
	require.NoError(t, err)
	return u
}

func (p *chaosProxy) setHealthy(healthy bool) {
	p.healthy.Store(healthy)
}

// dropConnections severs every connection to the proxy, which takes the
// gateway's multiplexed websocket to coderd down with it.
func (p *chaosProxy) dropConnections() {
	p.listener.sever()
}

// disconnect makes coderd both unreachable and unavailable, then waits for the
// gateway to observe the loss of its DRPC connection.
func (p *chaosProxy) disconnect(ctx context.Context, t *testing.T, gateway *testStandaloneGateway) {
	t.Helper()

	p.setHealthy(false)
	p.dropConnections()
	gateway.requireEventualStatus(ctx, t, readyzPath, http.StatusServiceUnavailable)
}

// mockAIStreamingUpstream is a mock upstream that starts a server-sent event
// stream, blocks until the test releases it, then finishes the stream. It lets
// a test drop the DRPC connection while a request is in flight and after its
// pre-flight authorization has already succeeded.
type mockAIStreamingUpstream struct {
	server *httptest.Server
	// started signals that the upstream has written the first chunk.
	started chan struct{}
	// release unblocks the upstream so it writes the rest of the stream.
	release chan struct{}
}

func newMockAIStreamingUpstream(t *testing.T) *mockAIStreamingUpstream {
	t.Helper()

	u := &mockAIStreamingUpstream{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	u.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockAIStreamFirstChunk))
		flusher.Flush()

		select {
		case u.started <- struct{}{}:
		default:
		}

		select {
		case <-u.release:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write([]byte(mockAIStreamFinalChunk))
		flusher.Flush()
	}))
	t.Cleanup(u.server.Close)
	return u
}

// hitCount reports how many requests the upstream has served so far.
func (u *mockAIUpstream) hitCount() int {
	return len(u.hits)
}

func (g *testStandaloneGateway) requireStatus(ctx context.Context, t *testing.T, path string, want int) {
	t.Helper()

	got, err := g.status(ctx, path)
	require.NoError(t, err)
	require.Equal(t, want, got, "unexpected status for %s", path)
}

// chatCompletion sends an LLM request through the gateway as the given Coder
// user token and returns the status and body.
func (g *testStandaloneGateway) chatCompletion(ctx context.Context, t *testing.T, token string) (int, []byte) {
	t.Helper()

	result := g.tryChatCompletion(ctx, token, testAIChatCompletionRequest)
	require.NoError(t, result.err)
	return result.status, result.body
}

// aiGatewayResponse is the outcome of an LLM request sent through a gateway.
type aiGatewayResponse struct {
	status int
	body   []byte
	err    error
}

// tryChatCompletion sends an LLM request to the gateway's own listener and
// reports transport errors instead of failing the test, so callers can assert
// on requests that never complete.
func (g *testStandaloneGateway) tryChatCompletion(ctx context.Context, token, requestBody string) aiGatewayResponse {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		g.baseURL.JoinPath(testAIProviderName, "/v1/chat/completions").String(), strings.NewReader(requestBody))
	if err != nil {
		return aiGatewayResponse{err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return aiGatewayResponse{err: err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	return aiGatewayResponse{status: resp.StatusCode, body: body, err: err}
}

// requireMockAIUpstreamResponse asserts the body is the upstream's fixed completion
// rather than a bridge-generated error.
func requireMockAIUpstreamResponse(t *testing.T, body []byte) {
	t.Helper()

	require.Equal(t, "standalone gateway response", gjson.GetBytes(body, "choices.0.message.content").String(), "body: %s", body)
	require.Equal(t, "gpt-4.1", gjson.GetBytes(body, "model").String(), "body: %s", body)
}

// requireInterceptions waits for coderd to have recorded want sessions for the
// user and returns them. Recording happens asynchronously over DRPC after the
// response is written, so the assertion has to be eventual.
func requireInterceptions(ctx context.Context, t *testing.T, coderd *testAIGatewayCoderd, want int) []codersdk.AIBridgeSession {
	t.Helper()

	var sessions []codersdk.AIBridgeSession
	require.Eventuallyf(t, func() bool {
		//nolint:gocritic // Owner role is needed to list every user's sessions.
		resp, err := coderd.client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Initiator: coderd.user.Username,
		})
		if err != nil {
			return false
		}
		sessions = resp.Sessions
		return len(sessions) == want
	}, testutil.WaitLong, testutil.IntervalFast, "expected %d recorded session(s), got %d", want, len(sessions))
	return sessions
}

// TestStandaloneGateway_ReconnectAfterDisconnect covers a coderd outage: the
// gateway keeps serving its HTTP listener, reports itself unready while the DRPC
// connection is gone, and resumes serving LLM traffic once coderd returns
// without any operator intervention.
func TestStandaloneGateway_ReconnectAfterDisconnect(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	coderd := setupAIGatewayCoderd(t)
	upstream := newMockAIUpstream(t)
	createTestAIProvider(ctx, t, coderd.client, upstream.server.URL)

	proxy := newChaosProxy(t, coderd.client.URL)
	gateway := startTestStandaloneGatewayWithURL(t, coderd, proxy.URL(t))
	gateway.requireEventualStatus(ctx, t, readyzPath, http.StatusOK)

	status, body := gateway.chatCompletion(ctx, t, coderd.userClient.SessionToken())
	require.Equal(t, http.StatusOK, status, "body: %s", body)

	proxy.disconnect(ctx, t, gateway)
	// The listener stays healthy so a load balancer keeps the replica alive
	// while only readiness withdraws it from rotation.
	gateway.requireStatus(ctx, t, healthzPath, http.StatusOK)

	proxy.setHealthy(true)
	gateway.requireEventualStatus(ctx, t, readyzPath, http.StatusOK)

	status, body = gateway.chatCompletion(ctx, t, coderd.userClient.SessionToken())
	require.Equal(t, http.StatusOK, status, "body: %s", body)
	requireMockAIUpstreamResponse(t, body)
	require.Equal(t, 2, upstream.hitCount())
	requireInterceptions(ctx, t, coderd, 2)
}

// TestStandaloneGateway_RequestWhileDisconnected pins the behavior of an LLM
// request that arrives while the gateway has no DRPC connection to coderd. The
// request is parked until the connection returns rather than failing fast.
//
// TODO(AIGOV-320): the RFC states such a request "fails with 503 if pre-flight
// DRPC calls cannot complete", which no caller can observe today. The gateway
// blocks in [aibridged.Server.Client] and only writes the 503 once the request
// context ends, and that context is not canceled when the caller disconnects
// because net/http keeps it live until the request body is consumed. An
// abandoned request therefore stays parked, and holds up graceful HTTP shutdown
// until the daemon reconnects. Either the daemon needs a pre-flight deadline or
// the RFC needs correcting.
func TestStandaloneGateway_RequestWhileDisconnected(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	coderd := setupAIGatewayCoderd(t)
	upstream := newMockAIUpstream(t)
	createTestAIProvider(ctx, t, coderd.client, upstream.server.URL)

	proxy := newChaosProxy(t, coderd.client.URL)
	gateway := startTestStandaloneGatewayWithURL(t, coderd, proxy.URL(t))
	gateway.requireEventualStatus(ctx, t, readyzPath, http.StatusOK)

	proxy.disconnect(ctx, t, gateway)

	// A request that arrives while disconnected does not fail fast; it blocks
	// until the caller gives up. The gateway has no server-side timeout on its
	// pre-flight DRPC calls, so the caller's deadline is the only bound.
	reqCtx, cancel := context.WithTimeout(ctx, testutil.IntervalSlow)
	defer cancel()
	abandoned := gateway.tryChatCompletion(reqCtx, coderd.userClient.SessionToken(), testAIChatCompletionRequest)
	require.ErrorIs(t, abandoned.err, context.DeadlineExceeded)
	require.Equal(t, 0, upstream.hitCount(), "a request must not reach the upstream before it is authorized")

	// A caller that keeps waiting is served as soon as the connection returns.
	responses := make(chan aiGatewayResponse, 1)
	go func() {
		responses <- gateway.tryChatCompletion(ctx, coderd.userClient.SessionToken(), testAIChatCompletionRequest)
	}()
	// The proxy is still unhealthy, so there is no path for this request to
	// complete. Confirming it stays parked is the assertion, not a workaround
	// for a race.
	select {
	case result := <-responses:
		t.Fatalf("request completed while disconnected: status=%d, err=%v", result.status, result.err)
	case <-time.After(testutil.IntervalSlow):
	}

	proxy.setHealthy(true)
	result := testutil.RequireReceive(ctx, t, responses)
	require.NoError(t, result.err)
	require.Equal(t, http.StatusOK, result.status, "body: %s", result.body)
	requireMockAIUpstreamResponse(t, result.body)
	require.Equal(t, 1, upstream.hitCount(), "only the request whose caller waited reaches the upstream")
}

// TestStandaloneGateway_InFlightRequestSurvivesDisconnect covers a request
// that has already passed pre-flight authorization when the DRPC connection to
// coderd drops. The response must complete for the caller instead of being torn
// down along with the connection.
func TestStandaloneGateway_InFlightRequestSurvivesDisconnect(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	coderd := setupAIGatewayCoderd(t)
	upstream := newMockAIStreamingUpstream(t)
	createTestAIProvider(ctx, t, coderd.client, upstream.server.URL)

	proxy := newChaosProxy(t, coderd.client.URL)
	gateway := startTestStandaloneGatewayWithURL(t, coderd, proxy.URL(t))
	gateway.requireEventualStatus(ctx, t, readyzPath, http.StatusOK)

	responses := make(chan aiGatewayResponse, 1)
	go func() {
		responses <- gateway.tryChatCompletion(ctx, coderd.userClient.SessionToken(), testAIStreamingChatCompletionRequest)
	}()
	testutil.RequireReceive(ctx, t, upstream.started)

	proxy.disconnect(ctx, t, gateway)
	close(upstream.release)

	result := testutil.RequireReceive(ctx, t, responses)
	require.NoError(t, result.err)
	require.Equal(t, http.StatusOK, result.status, "body: %s", result.body)
	require.Contains(t, string(result.body), "first half")
	require.Contains(t, string(result.body), "second half")

	// The recording RPCs that follow the response share the connection that was
	// dropped, so this interception's usage rows are expected to be lost. Only
	// the caller-visible outcome is asserted.
}
