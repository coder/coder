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
	"os"
	"path/filepath"
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

// withAIGatewayCoderdOptions mutates coderd's options.
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

// startUnreachableCoderd starts a coderd stub that answers 503 to every
// request, so the gateway daemon keeps retrying its control connection instead
// of completing. It returns the stub's URL.
func startUnreachableCoderd(t *testing.T) string {
	t.Helper()

	coderSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(coderSrv.Close)
	return coderSrv.URL
}

// TestAIGatewayStartE2E drives every part of the standalone gateway plumbing
// once through public surface only: the CLI starts with flags, connects to
// coderd with a gateway key, reports health and readiness, proxies an LLM
// request from a real client to a real upstream, records the interception in
// coderd, and shuts down cleanly.
func TestAIGatewayStartE2E(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	// Given: a coderd entitled for the AI Gateway, with a key and a provider.
	dep := setupAIGatewayDeployment(ctx, t)

	// When: the gateway starts against that coderd.
	baseURL, waiter := startAIGatewayCommand(ctx, t, dep.client.URL.String(), dep.key)

	// Then: liveness holds as soon as the listener is bound, and readiness
	// follows the DRPC connection and the provider load.
	requireAIGatewayStatus(ctx, t, baseURL+aiGatewayHealthzPath, http.StatusOK)
	requireEventualAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusOK)

	// When: a user sends an LLM request to the gateway.
	result := postChatCompletionAndRead(ctx, baseURL+aiGatewayChatCompletionPath,
		dep.userClient.SessionToken(), aiGatewayChatCompletionRequest)

	// Then: the upstream's response reaches the caller.
	require.NoError(t, result.err)
	require.Equal(t, http.StatusOK, result.status, "body: %s", result.body)
	require.Contains(t, string(result.body), "standalone gateway e2e response")
	require.Equal(t, int32(1), dep.upstreamHits.Load())

	// Then: coderd records the interception, attributed to that user.
	sessions := requireAIGatewaySessions(ctx, t, dep, 1)
	require.Equal(t, dep.user.Username, sessions[0].Initiator.Username)

	// When: the command is canceled.
	waiter.Cancel()
	// Then: it exits cleanly.
	require.NoError(t, waiter.Wait())
}

// TestAIGatewayStartE2E_InvalidKey covers the fatal error plumbing: a gateway
// started with a key coderd rejects must exit with the rejection instead of
// retrying forever.
func TestAIGatewayStartE2E_InvalidKey(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := setupAIGatewayCoderdenttestDeployment(t)

	// When: the gateway starts with invalid key.
	inv, _ := newCLI(t,
		"ai-gateway", "start",
		"--url", client.URL.String(),
		"--key", "not-a-valid-key",
		"--http-address", "127.0.0.1:0",
	)
	inv = inv.WithContext(ctx)
	waiter := clitest.StartWithWaiter(t, inv)

	// Then: it exits with error without retrying.
	require.ErrorContains(t, waiter.Wait(), "AI Gateway key invalid")
}

// TestAIGatewayStart_HealthBeforeReady covers the split between liveness and
// readiness: the listener serves /healthz as soon as it is bound, while
// /readyz stays 503 until the daemon reaches coderd.
func TestAIGatewayStart_HealthBeforeReady(t *testing.T) {
	t.Parallel()

	// Given: a coderd that always answers 503, so the daemon keeps retrying.
	coderURL := startUnreachableCoderd(t)

	ctx := testutil.Context(t, testutil.WaitShort)
	// When: the gateway starts and binds its listener.
	baseURL, _ := startAIGatewayCommand(ctx, t, coderURL, "test-key")

	// Then: healthz is already 200 while readyz stays 503.
	// The startup log line is emitted after the bind, so no retry is needed.
	requireAIGatewayStatus(ctx, t, baseURL+aiGatewayHealthzPath, http.StatusOK)
	requireAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusServiceUnavailable)
}

// TestAIGatewayStartE2E_RevokedKey covers revocation of a key that is already
// in use. coderd closes the active session, the gateway redials, and the 401
// on that redial must terminate the command.
func TestAIGatewayStartE2E_RevokedKey(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	// Given: a ready gateway, and a coderd whose key check ticker the test drives.
	keyCheck := make(chan time.Time, 1)
	dep := setupAIGatewayDeployment(ctx, t, withAIGatewayCoderdOptions(func(opts *coderdenttest.Options) {
		opts.Options.NewTicker = func(time.Duration) (<-chan time.Time, func()) {
			return keyCheck, func() {}
		}
	}))

	baseURL, waiter := startAIGatewayCommand(ctx, t, dep.client.URL.String(), dep.key)
	requireEventualAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusOK)

	// When: the key is deleted and coderd runs its check.
	//nolint:gocritic // Owner role is needed for gateway key management.
	require.NoError(t, dep.client.DeleteAIGatewayKey(ctx, dep.keyID))
	keyCheck <- time.Now()

	// Then: the 401 on the gateway's redial terminates the command.
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

// disconnect makes coderd both unreachable and unavailable. Closing the proxy's
// connections takes the gateway's multiplexed websocket down with them.
func (p *chaosProxy) disconnect() {
	p.setHealthy(false)
	p.listener.closeConns()
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
func TestAIGatewayStartE2E_ReconnectAfterDisconnect(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	dep := setupAIGatewayDeployment(ctx, t)

	// Given: a ready gateway, reaching coderd through a chaos proxy.
	proxy := newChaosProxy(t, dep.client.URL)
	baseURL, _ := startAIGatewayCommand(ctx, t, proxy.srv.URL, dep.key)
	requireEventualAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusOK)

	// When: a request arrives while coderd is reachable.
	before := postChatCompletionAndRead(ctx, baseURL+aiGatewayChatCompletionPath,
		dep.userClient.SessionToken(), aiGatewayChatCompletionRequest)

	// Then: it is served and recorded.
	require.NoError(t, before.err)
	require.Equal(t, http.StatusOK, before.status, "body: %s", before.body)
	requireAIGatewaySessions(ctx, t, dep, 1)

	// When: coderd becomes unreachable.
	proxy.disconnect()

	// Then: readiness withdraws.
	requireEventualAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusServiceUnavailable)

	// When: coderd is reachable again.
	proxy.setHealthy(true)

	// Then: readiness recovers with no intervention.
	requireEventualAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusOK)

	// Then: a new request is served and both interceptions are recorded, so the
	// provider cache and the recorder recovered with the connection.
	after := postChatCompletionAndRead(ctx, baseURL+aiGatewayChatCompletionPath,
		dep.userClient.SessionToken(), aiGatewayChatCompletionRequest)
	require.NoError(t, after.err)
	require.Equal(t, http.StatusOK, after.status, "body: %s", after.body)
	require.Contains(t, string(after.body), "standalone gateway e2e response")
	require.Equal(t, int32(2), dep.upstreamHits.Load())
	requireAIGatewaySessions(ctx, t, dep, 2)
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

	// Given: a ready gateway that then loses its connection to coderd.
	proxy := newChaosProxy(t, dep.client.URL)
	baseURL, _ := startAIGatewayCommand(ctx, t, proxy.srv.URL, dep.key)
	requireEventualAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusOK)
	proxy.disconnect()
	requireEventualAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusServiceUnavailable)

	// When: a request arrives while the gateway has no connection to coderd.
	responses := make(chan aiGatewayResponse, 1)
	go func() {
		responses <- postChatCompletionAndRead(ctx, baseURL+aiGatewayChatCompletionPath,
			dep.userClient.SessionToken(), aiGatewayChatCompletionRequest)
	}()

	// Then: it does not fail fast, and it does not reach the upstream before it
	// is authorized which requires a connection to coderd.
	select {
	case result := <-responses:
		t.Fatalf("request completed while disconnected: status=%d, err=%v", result.status, result.err)
	case <-time.After(testutil.IntervalMedium):
	}
	require.Equal(t, int32(0), dep.upstreamHits.Load(), "a request must not reach the upstream before it is authorized")

	// When: coderd returns. Then: the parked request is served.
	proxy.setHealthy(true)
	result := testutil.RequireReceive(ctx, t, responses)
	require.NoError(t, result.err)
	require.Equal(t, http.StatusOK, result.status, "body: %s", result.body)
	require.Contains(t, string(result.body), "standalone gateway e2e response")
	require.Equal(t, int32(1), dep.upstreamHits.Load(), "the parked request reaches the upstream once it is authorized")
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

	// Given: a ready gateway, and a streaming request whose first chunk the
	// caller has already received.
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
	stream := bufio.NewReader(resp.Body)
	require.Equal(t, "first half", readAIGatewayStreamDelta(t, stream))

	// When: the DRPC connection drops and the upstream finishes the stream.
	proxy.disconnect()
	requireEventualAIGatewayStatus(ctx, t, baseURL+aiGatewayReadyzPath, http.StatusServiceUnavailable)
	close(release)

	// Then: the rest of the stream reaches the caller, in order.
	require.Equal(t, " second half", readAIGatewayStreamDelta(t, stream))
	require.Equal(t, "[DONE]", readAIGatewayStreamEvent(t, stream), "the stream must be terminated")
	require.Equal(t, int32(1), dep.upstreamHits.Load(), "the request must reach the upstream exactly once")

	// The recording RPCs that follow the response share the connection that was
	// dropped, so this interception's usage rows are expected to be lost. Only
	// the caller-visible outcome is asserted.
}

// TestAIGatewayStart_ConfigYAML verifies that a YAML file supplied via
// --config (CODER_CONFIG_PATH) configures the running standalone Gateway.
// It enables the inherited Prometheus listener through YAML and asserts that the
// listener comes up.
func TestAIGatewayStart_ConfigYAML(t *testing.T) {
	t.Parallel()

	// Fake coderd that answers 503 so the daemon keeps retrying to connect.
	coderURL := startUnreachableCoderd(t)
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	err := os.WriteFile(configFile, []byte(
		"introspection:\n  prometheus:\n    enable: true\n    address: 127.0.0.1:0\n",
	), 0o600)
	require.NoError(t, err)

	ctx := testutil.Context(t, testutil.WaitShort)
	inv, _ := newCLI(t,
		"ai-gateway", "start",
		"--url", coderURL,
		"--key", "test-key",
		"--http-address", "127.0.0.1:0",
		"--config", configFile,
	)
	inv = inv.WithContext(ctx)
	pty := ptytest.New(t).Attach(inv)
	waiter := clitest.StartWithWaiter(t, inv)

	// The Prometheus listener is only started when prometheus.enable
	// option is set which is enabled through YAML.
	promLine := pty.ExpectRegexMatch(ctx, `http server listening\s+addr=[0-9.]+:[0-9]+\s+name=prometheus`)
	matches := regexp.MustCompile(`addr=([0-9.]+:[0-9]+)`).FindStringSubmatch(promLine)
	require.Len(t, matches, 2, "prometheus address not found in startup log: %q", promLine)
	promURL := "http://" + matches[1] + "/metrics"
	requireAIGatewayStatus(ctx, t, promURL, http.StatusOK)

	waiter.Cancel()
	require.NoError(t, waiter.Wait())
}

// TestAIGatewayStart_ConfigYAML_Invalid verifies that a YAML file with an
// unknown option fails the command with a descriptive error.
func TestAIGatewayStart_ConfigYAML_Invalid(t *testing.T) {
	t.Parallel()

	configFile := filepath.Join(t.TempDir(), "config.yaml")
	err := os.WriteFile(configFile, []byte(
		"introspection:\n  prometheus:\n    unknown_field: true\n",
	), 0o600)
	require.NoError(t, err)

	inv, _ := newCLI(t,
		"ai-gateway", "start",
		"--key", "test-key",
		"--config", configFile,
	)

	err = inv.Run()
	require.ErrorContains(t, err, `unknown option "introspection.prometheus.unknown_field"`)
}
