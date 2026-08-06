//go:build !slim

package cli_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

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
// against a real coderd and only observe the public surface: CLI flags, log
// output, the gateway's HTTP endpoints, coderd's API, and the command's exit
// error. Each part of the standalone plumbing executes at least once. Detailed
// behavior (readiness transitions, reconnect semantics, shutdown ordering) is
// covered by the internal tests in aigatewaystart_internal_test.go and the
// reconnection tests, which construct the gateway directly.

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

// aiGatewayDeployment is a coderd entitled for the AI Gateway, with a gateway key, a
// configured provider backed by a mock upstream, and a member user whose
// session token authenticates LLM traffic.
type aiGatewayDeployment struct {
	client       *codersdk.Client
	userClient   *codersdk.Client
	user         codersdk.User
	key          string
	upstreamHits *atomic.Int32
}

func setupAIGatewayDeployment(ctx context.Context, t *testing.T) *aiGatewayDeployment {
	t.Helper()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	client, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureAIBridge: 1},
		},
	})

	//nolint:gocritic // Owner role is needed for gateway key management.
	key, err := client.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: "e2e"})
	require.NoError(t, err)

	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(aiGatewayUpstreamResponse))
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

func getAIGatewayStatus(ctx context.Context, t *testing.T, url string) int {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode
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
	require.Equal(t, http.StatusOK, getAIGatewayStatus(ctx, t, baseURL+"/healthz"))
	require.Eventually(t, func() bool {
		return getAIGatewayStatus(ctx, t, baseURL+"/readyz") == http.StatusOK
	}, testutil.WaitLong, testutil.IntervalFast)

	// One LLM request through the gateway's own listener.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/openai/v1/chat/completions", strings.NewReader(aiGatewayChatCompletionRequest))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+dep.userClient.SessionToken())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	require.Contains(t, string(body), "standalone gateway e2e response")
	require.Equal(t, int32(1), dep.upstreamHits.Load())

	// The interception is recorded in coderd. Recording is asynchronous, so
	// the assertion has to be eventual.
	require.Eventually(t, func() bool {
		//nolint:gocritic // Owner role is needed to list every user's sessions.
		sessions, err := dep.client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Initiator: dep.user.Username,
		})
		return err == nil && len(sessions.Sessions) == 1
	}, testutil.WaitLong, testutil.IntervalFast)

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
	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	client, _ := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureAIBridge: 1},
		},
	})

	inv, _ := newCLI(t,
		"ai-gateway", "start",
		"--url", client.URL.String(),
		"--key", "not-a-valid-key",
		"--http-address", "127.0.0.1:0",
	)
	inv = inv.WithContext(ctx)
	waiter := clitest.StartWithWaiter(t, inv)
	waiter.RequireContains("AI Gateway key invalid")
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
	require.Equal(t, http.StatusOK, getAIGatewayStatus(ctx, t, baseURL+"/healthz"))
	require.Equal(t, http.StatusServiceUnavailable, getAIGatewayStatus(ctx, t, baseURL+"/readyz"))
}
