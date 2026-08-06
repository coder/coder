//go:build !slim

package cli_test

import (
	"bufio"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// The end-to-end tests in this file run the real `ai-gateway start` command
// against a real coderd and only observe the public surface.
//
// Readiness state transitions, shutdown ordering, and the classification of
// dial errors are asserted directly, using the internals of gateway struct in
// aigatewaystart_internal_test.go.

// aiGatewayChatCompletionRequest is the LLM request sent through the gateway. The
// model is asserted against the recorded interception.
const aiGatewayChatCompletionRequest = `{"messages":[{"role":"user","content":"standalone gateway e2e"}],"model":"gpt-4.1"}`

// aiGatewayUpstreamResponse is the fixed completion the mock upstream LLM API
// returns, so the test can recognize it in the gateway's response.
const aiGatewayUpstreamResponse = `{
  "id": "chatcmpl-e2e",
  "object": "chat.completion",
  "created": 1753343279,
  "model": "gpt-4.1",
  "choices": [
    {
      "index": 0,
      "message": {"role": "assistant", "content": "standalone gateway e2e response"},
      "finish_reason": "stop"
    }
  ],
  "usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
}`

const (
	aiGatewayStreamFirstChunk = `data: {"id":"chatcmpl-e2e","object":"chat.completion.chunk","created":1753343279,"model":"gpt-4.1","choices":[{"index":0,"delta":{"role":"assistant","content":"first half"},"finish_reason":null}]}` + "\n\n"
	aiGatewayStreamFinalChunk = `data: {"id":"chatcmpl-e2e","object":"chat.completion.chunk","created":1753343279,"model":"gpt-4.1","choices":[{"index":0,"delta":{"content":" second half"},"finish_reason":"stop"}]}` + "\n\n" + "data: [DONE]\n\n"
)

const (
	aiGatewayHealthzPath = "/healthz"
	aiGatewayReadyzPath  = "/readyz"
	// aiGatewayChatCompletionPath is the LLM route on the gateway's own
	// listener. The aibridge mux routes by provider name, so the first segment
	// is the name the provider is registered under.
	aiGatewayChatCompletionPath = "/openai/v1/chat/completions"
)

// aiGatewayDeployment is a coderd entitled for the AI Gateway, with a gateway key, a
// configured provider backed by a mock upstream, and a member user whose
// session token authenticates LLM traffic.
type aiGatewayDeployment struct {
	client       *codersdk.Client
	userClient   *codersdk.Client
	user         codersdk.User
	key          string
	keyID        uuid.UUID
	upstreamHits *atomic.Int32
}

func setupAIGatewayCoderdenttestDeployment(t *testing.T, mutate ...func(*coderdenttest.Options)) (*codersdk.Client, codersdk.CreateFirstUserResponse) {
	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	opts := &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureAIBridge: 1},
		},
	}
	for _, m := range mutate {
		m(opts)
	}
	return coderdenttest.New(t, opts)
}

// aiGatewayDeploymentConfig is the deployment shape a test needs.
type aiGatewayDeploymentConfig struct {
	// upstream serves the mock LLM API the provider points at.
	upstream http.HandlerFunc
	// coderdOptions mutates coderd's options before it starts.
	coderdOptions func(*coderdenttest.Options)
}

type aiGatewayDeploymentOption func(*aiGatewayDeploymentConfig)

// withAIGatewayUpstream replaces the mock LLM API. Requests are still counted
// in aiGatewayDeployment.upstreamHits.
func withAIGatewayUpstream(handler http.HandlerFunc) aiGatewayDeploymentOption {
	return func(cfg *aiGatewayDeploymentConfig) {
		cfg.upstream = handler
	}
}

// withAIGatewayCoderdOptions mutates coderd's options, for example to replace
// the ticker that drives its gateway key checks.
func withAIGatewayCoderdOptions(mutate func(*coderdenttest.Options)) aiGatewayDeploymentOption {
	return func(cfg *aiGatewayDeploymentConfig) {
		cfg.coderdOptions = mutate
	}
}

func setupAIGatewayDeployment(ctx context.Context, t *testing.T, opts ...aiGatewayDeploymentOption) *aiGatewayDeployment {
	t.Helper()

	cfg := aiGatewayDeploymentConfig{
		upstream: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(aiGatewayUpstreamResponse))
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	var coderdMutations []func(*coderdenttest.Options)
	if cfg.coderdOptions != nil {
		coderdMutations = append(coderdMutations, cfg.coderdOptions)
	}
	client, firstUser := setupAIGatewayCoderdenttestDeployment(t, coderdMutations...)

	//nolint:gocritic // Owner role is needed for gateway key management.
	key, err := client.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: "e2e"})
	require.NoError(t, err)

	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		cfg.upstream(w, r)
	}))
	t.Cleanup(upstream.Close)

	//nolint:gocritic // Owner role is needed for provider management.
	_, err = client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
		Type:    codersdk.AIProviderTypeOpenAI,
		Name:    "openai",
		Enabled: true,
		BaseURL: upstream.URL,
		APIKeys: []string{"sk-e2e"},
	})
	require.NoError(t, err)

	userClient, user := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

	return &aiGatewayDeployment{
		client:       client,
		userClient:   userClient,
		user:         user,
		key:          key.Key,
		keyID:        key.ID,
		upstreamHits: &hits,
	}
}

// startAIGatewayCommand runs `ai-gateway start` and returns the base URL of
// its HTTP listener, discovered from the startup log line, together with the
// command's error waiter.
func startAIGatewayCommand(ctx context.Context, t *testing.T, coderURL, key string) (string, *clitest.ErrorWaiter) {
	t.Helper()

	inv, _ := newCLI(t,
		"ai-gateway", "start",
		"--url", coderURL,
		"--key", key,
		"--http-address", "127.0.0.1:0",
	)
	inv = inv.WithContext(ctx)
	pty := ptytest.New(t).Attach(inv)
	waiter := clitest.StartWithWaiter(t, inv)

	// Extract bound address from the startup log.
	pty.ExpectMatch(ctx, "standalone AI Gateway listening")
	line := pty.ReadLine(ctx)
	matches := regexp.MustCompile(`address=([0-9.]+:[0-9]+)`).FindStringSubmatch(line)
	require.Len(t, matches, 2, "listener address not found in startup log: %q", line)
	return "http://" + matches[1], waiter
}

// aiGatewayStatus probes probeURL and reports transport errors.
func aiGatewayStatus(ctx context.Context, probeURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// requireAIGatewayStatus asserts the status of a single probe.
func requireAIGatewayStatus(ctx context.Context, t *testing.T, probeURL string, want int) {
	t.Helper()

	got, err := aiGatewayStatus(ctx, probeURL)
	require.NoError(t, err)
	require.Equal(t, want, got, "unexpected status for %s", probeURL)
}

// requireEventualAIGatewayStatus waits for probeURL to return want, tolerating
// transport errors while the gateway converges.
func requireEventualAIGatewayStatus(ctx context.Context, t *testing.T, probeURL string, want int) {
	t.Helper()

	require.Eventuallyf(t, func() bool {
		got, err := aiGatewayStatus(ctx, probeURL)
		return err == nil && got == want
	}, testutil.WaitLong, testutil.IntervalFast, "%s never returned %d", probeURL, want)
}

// TestAIGatewayStartE2E drives every part of the standalone gateway plumbing
// once through public surface only: the CLI starts with flags, connects to
// coderd with a gateway key, reports health and readiness, proxies an LLM
// request from a real client to a real upstream, records the interception in
// coderd, and shuts down cleanly.
func TestAIGatewayStartE2E(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	dep := setupAIGatewayDeployment(ctx, t)

	baseURL, waiter := startAIGatewayCommand(ctx, t, dep.client.URL.String(), dep.key)

	// Liveness holds as soon as the listener is up; readiness follows once the
	// DRPC connection is established and providers are loaded.
	requireAIGatewayStatus(ctx, t, baseURL+aiGatewayHealthzPath, http.StatusOK)
	requireEventualAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusOK)

	// One LLM request through the gateway's own listener.
	result := postChatCompletionAndRead(ctx, baseURL+aiGatewayChatCompletionPath,
		dep.userClient.SessionToken(), aiGatewayChatCompletionRequest)
	require.NoError(t, result.err)
	require.Equal(t, http.StatusOK, result.status, "body: %s", result.body)
	require.Contains(t, string(result.body), "standalone gateway e2e response")
	require.Equal(t, int32(1), dep.upstreamHits.Load())

	// The interception is recorded in coderd.
	sessions := requireAIGatewaySessions(ctx, t, dep, 1)
	require.Equal(t, dep.user.Username, sessions[0].Initiator.Username)

	// Graceful shutdown: canceling the command must produce a clean exit.
	waiter.Cancel()
	require.NoError(t, waiter.Wait())
}

// TestAIGatewayStartE2E_InvalidKey covers the fatal error plumbing: a gateway
// started with a key coderd rejects must exit with the rejection instead of
// retrying forever.
func TestAIGatewayStartE2E_InvalidKey(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := setupAIGatewayCoderdenttestDeployment(t)

	inv, _ := newCLI(t,
		"ai-gateway", "start",
		"--url", client.URL.String(),
		"--key", "not-a-valid-key",
		"--http-address", "127.0.0.1:0",
	)
	inv = inv.WithContext(ctx)
	waiter := clitest.StartWithWaiter(t, inv)
	require.ErrorContains(t, waiter.Wait(), "AI Gateway key invalid")
}

// TestAIGatewayStart_HealthBeforeReady covers the split between liveness and
// readiness: the listener serves /healthz as soon as it is bound, while
// /readyz stays 503 until the daemon reaches coderd.
func TestAIGatewayStart_HealthBeforeReady(t *testing.T) {
	t.Parallel()

	// Fake coderd that answers 503 so the daemon keeps retrying to connect.
	coderSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(coderSrv.Close)

	ctx := testutil.Context(t, testutil.WaitShort)
	baseURL, _ := startAIGatewayCommand(ctx, t, coderSrv.URL, "test-key")

	// The startup log line is emitted after the listener is bound, so no retry
	// loop is needed.
	requireAIGatewayStatus(ctx, t, baseURL+aiGatewayHealthzPath, http.StatusOK)
	requireAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusServiceUnavailable)
}

// TestAIGatewayStartE2E_RevokedKey covers revocation of a key that is already
// in use. coderd closes the active session, the gateway redials, and the 401
// on that redial must terminate the command.
func TestAIGatewayStartE2E_RevokedKey(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	keyCheck := make(chan time.Time, 1)
	dep := setupAIGatewayDeployment(ctx, t, withAIGatewayCoderdOptions(func(opts *coderdenttest.Options) {
		opts.Options.NewTicker = func(time.Duration) (<-chan time.Time, func()) {
			return keyCheck, func() {}
		}
	}))

	baseURL, waiter := startAIGatewayCommand(ctx, t, dep.client.URL.String(), dep.key)
	requireEventualAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusOK)

	//nolint:gocritic // Owner role is needed for gateway key management.
	require.NoError(t, dep.client.DeleteAIGatewayKey(ctx, dep.keyID))
	keyCheck <- time.Now()

	require.ErrorContains(t, waiter.Wait(), "AI Gateway key invalid")
}

// connTrackingListener records the connections it accepts so a test can close
// them while the listener keeps accepting new ones.
//
// It is needed because the gateway's connection to coderd is a websocket, and
// httputil.ReverseProxy serves the resulting 101 by hijacking its inbound
// connection and splicing bytes for the life of the upgrade. httptest.Server
// deletes a connection from its tracking set on http.StateHijacked, so
// CloseClientConnections can no longer reach it, and the handler that would
// observe the proxy being unhealthy never runs again for an established
// websocket. Accept happens before any of that, so the raw net.Conn kept here
// stays closable.
type connTrackingListener struct {
	net.Listener

	mu    sync.Mutex
	conns []net.Conn
}

func (l *connTrackingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.conns = append(l.conns, conn)
	return conn, nil
}

// closeConns closes every connection accepted so far and stops tracking them,
// so connections established afterwards survive. The listener itself stays
// open.
func (l *connTrackingListener) closeConns() {
	l.mu.Lock()
	conns := l.conns
	l.conns = nil
	l.mu.Unlock()

	for _, conn := range conns {
		_ = conn.Close()
	}
}

// chaosProxy sits between the standalone gateway and coderd so tests can
// simulate network failures and coderd unavailability. While unhealthy
// it answers 503, which the connect loop classifies as transient and retries.
type chaosProxy struct {
	srv      *httptest.Server
	listener *connTrackingListener
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
	p.listener = &connTrackingListener{Listener: p.srv.Listener}
	p.srv.Listener = p.listener
	p.srv.Start()
	t.Cleanup(p.srv.Close)
	return p
}

func (p *chaosProxy) setHealthy(healthy bool) {
	p.healthy.Store(healthy)
}

// disconnect makes coderd both unreachable and unavailable, then waits for the
// gateway to observe the loss of its DRPC connection. Closing the proxy's
// connections takes the gateway's multiplexed websocket down with them.
//
// Readiness is the only signal a caller has that the gateway noticed, so it is
// used to sequence the test. That readiness follows the DRPC connection at all
// is asserted by TestStandaloneGatewayHealthAndReadiness.
func (p *chaosProxy) disconnect(ctx context.Context, t *testing.T, gatewayBaseURL string) {
	t.Helper()

	p.setHealthy(false)
	p.listener.closeConns()
	requireEventualAIGatewayStatus(ctx, t, gatewayBaseURL+aiGatewayReadyzPath, http.StatusServiceUnavailable)
}

// aiGatewayResponse is the outcome of an LLM request sent through a gateway.
type aiGatewayResponse struct {
	status int
	body   []byte
	err    error
}

// postChatCompletion sends an LLM request and returns the response with its body
// unread. The caller owns the body.
//
//nolint:bodyclose // The caller owns and closes the body.
func postChatCompletion(ctx context.Context, endpoint, token, requestBody string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(requestBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	return http.DefaultClient.Do(req)
}

// postChatCompletionAndRead sends an LLM request, reads the whole response, and
// reports transport errors.
func postChatCompletionAndRead(ctx context.Context, endpoint, token, requestBody string) aiGatewayResponse {
	resp, err := postChatCompletion(ctx, endpoint, token, requestBody)
	if err != nil {
		return aiGatewayResponse{err: err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	return aiGatewayResponse{status: resp.StatusCode, body: body, err: err}
}

// requireAIGatewaySessions waits for coderd to have recorded 'want' sessions for
// the deployment's user and returns them.
func requireAIGatewaySessions(ctx context.Context, t *testing.T, dep *aiGatewayDeployment, want int) []codersdk.AIBridgeSession {
	t.Helper()

	var (
		sessions []codersdk.AIBridgeSession
		lastErr  error
	)
	require.Eventuallyf(t, func() bool {
		//nolint:gocritic // Owner (or Auditor) role is needed to read sessions.
		resp, err := dep.client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Initiator: dep.user.Username,
		})
		lastErr = err
		if err != nil {
			return false
		}
		sessions = resp.Sessions
		return len(sessions) == want
	}, testutil.WaitLong, testutil.IntervalFast,
		"expected %d recorded session(s), got %d, last error: %v", want, len(sessions), lastErr)
	return sessions
}

// TestAIGatewayStartE2E_ReconnectAfterDisconnect covers a coderd outage: the
// gateway serves LLM traffic again once coderd returns, without any operator
// intervention, which requires the provider cache and the recorder to recover
// alongside the DRPC connection.
//
// The readiness transitions themselves are asserted by
// TestStandaloneGatewayHealthAndReadiness and are used here only to sequence
// the outage.
func TestAIGatewayStartE2E_ReconnectAfterDisconnect(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	dep := setupAIGatewayDeployment(ctx, t)

	proxy := newChaosProxy(t, dep.client.URL)
	baseURL, _ := startAIGatewayCommand(ctx, t, proxy.srv.URL, dep.key)
	requireEventualAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusOK)

	proxy.disconnect(ctx, t, baseURL)
	proxy.setHealthy(true)
	requireEventualAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusOK)

	result := postChatCompletionAndRead(ctx, baseURL+aiGatewayChatCompletionPath,
		dep.userClient.SessionToken(), aiGatewayChatCompletionRequest)
	require.NoError(t, result.err)
	require.Equal(t, http.StatusOK, result.status, "body: %s", result.body)
	require.Contains(t, string(result.body), "standalone gateway e2e response")
	require.Equal(t, int32(1), dep.upstreamHits.Load())
	requireAIGatewaySessions(ctx, t, dep, 1)
}

// TestAIGatewayStartE2E_RequestWhileDisconnected pins the behavior of an LLM
// request that arrives while the gateway has no DRPC connection to coderd. The
// request is parked until the connection returns rather than failing fast,
// because the pre-flight calls block in [aibridged.Server.Client] with the
// caller's context as the only bound.
func TestAIGatewayStartE2E_RequestWhileDisconnected(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	dep := setupAIGatewayDeployment(ctx, t)

	proxy := newChaosProxy(t, dep.client.URL)
	baseURL, _ := startAIGatewayCommand(ctx, t, proxy.srv.URL, dep.key)
	requireEventualAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusOK)

	proxy.disconnect(ctx, t, baseURL)

	// A request that arrives while disconnected does not fail fast; it blocks
	// until the caller gives up. The gateway has no server-side timeout on its
	// pre-flight DRPC calls, so the caller's deadline is the only bound.
	reqCtx, cancel := context.WithTimeout(ctx, testutil.IntervalSlow)
	defer cancel()
	abandoned := postChatCompletionAndRead(reqCtx, baseURL+aiGatewayChatCompletionPath,
		dep.userClient.SessionToken(), aiGatewayChatCompletionRequest)
	require.ErrorIs(t, abandoned.err, context.DeadlineExceeded)
	require.Equal(t, int32(0), dep.upstreamHits.Load(), "a request must not reach the upstream before it is authorized")

	// A caller that keeps waiting is served as soon as the connection returns.
	responses := make(chan aiGatewayResponse, 1)
	go func() {
		responses <- postChatCompletionAndRead(ctx, baseURL+aiGatewayChatCompletionPath,
			dep.userClient.SessionToken(), aiGatewayChatCompletionRequest)
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
	require.Contains(t, string(result.body), "standalone gateway e2e response")
	require.Equal(t, int32(1), dep.upstreamHits.Load(), "only the request whose caller waited reaches the upstream")
}

// readAIGatewayStreamEvent returns the payload of the next server-sent event,
// blocking until the gateway forwards it.
func readAIGatewayStreamEvent(t *testing.T, stream *bufio.Reader) string {
	t.Helper()

	for {
		line, err := stream.ReadString('\n')
		require.NoError(t, err, "read stream event")
		payload, ok := strings.CutPrefix(strings.TrimSuffix(line, "\n"), "data: ")
		if !ok {
			// Blank line separating events.
			continue
		}
		return payload
	}
}

// readAIGatewayStreamDelta returns the content delta of the next event.
func readAIGatewayStreamDelta(t *testing.T, stream *bufio.Reader) string {
	t.Helper()

	payload := readAIGatewayStreamEvent(t, stream)
	return gjson.Get(payload, "choices.0.delta.content").String()
}

// blockingStreamAIGatewayUpstream returns a mock upstream that starts a
// server-sent event stream, blocks until release is closed, then finishes the
// stream. It lets a test drop the DRPC connection while a request is in flight,
// after its pre-flight authorization has already succeeded and after the caller
// has received part of the response.
func blockingStreamAIGatewayUpstream(release <-chan struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(aiGatewayStreamFirstChunk))
		flusher.Flush()

		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write([]byte(aiGatewayStreamFinalChunk))
		flusher.Flush()
	}
}

// TestAIGatewayStartE2E_InFlightRequestSurvivesDisconnect covers a request that
// has already passed pre-flight authorization and had part of its response
// delivered when the DRPC connection to coderd drops. The rest of the stream
// must reach the caller instead of being torn down along with the connection.
func TestAIGatewayStartE2E_InFlightRequestSurvivesDisconnect(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	release := make(chan struct{})
	dep := setupAIGatewayDeployment(ctx, t, withAIGatewayUpstream(blockingStreamAIGatewayUpstream(release)))

	proxy := newChaosProxy(t, dep.client.URL)
	baseURL, _ := startAIGatewayCommand(ctx, t, proxy.srv.URL, dep.key)
	requireEventualAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusOK)

	req, err := sjson.Set(aiGatewayChatCompletionRequest, "stream", true)
	require.NoError(t, err)
	resp, err := postChatCompletion(ctx, baseURL+aiGatewayChatCompletionPath,
		dep.userClient.SessionToken(), req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Reading the first delta proves the gateway already forwarded part of the
	// response, so it can no longer substitute an error for it. The upstream is
	// still blocked, so the rest of the stream depends on what happens next.
	stream := bufio.NewReader(resp.Body)
	require.Equal(t, "first half", readAIGatewayStreamDelta(t, stream))

	proxy.disconnect(ctx, t, baseURL)
	close(release)

	require.Equal(t, " second half", readAIGatewayStreamDelta(t, stream))
	require.Equal(t, "[DONE]", readAIGatewayStreamEvent(t, stream), "the stream must be terminated")
	require.Equal(t, int32(1), dep.upstreamHits.Load(), "the request must reach the upstream exactly once")

	// The recording RPCs that follow the response share the connection that was
	// dropped, so this interception's usage rows are expected to be lost. Only
	// the caller-visible outcome is asserted.
}
