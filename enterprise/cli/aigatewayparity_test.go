//go:build !slim

package cli_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// gatewayMode is one of the two ways the AI Gateway can be deployed.
type gatewayMode struct {
	name string
	// endpoint starts the gateway for this mode against its own coderd and
	// returns the chat completion URL LLM clients send requests to.
	endpoint func(ctx context.Context, t *testing.T, dep *e2eDeployment) string
}

// gatewayOutcome is everything a caller and an auditor can observe about one
// LLM request served by a gateway.
type gatewayOutcome struct {
	status  int
	body    []byte
	session codersdk.AIBridgeSession
}

// TestGatewayParityE2E_EmbeddedVsStandalone asserts the standalone AI Gateway
// is interchangeable with the daemon embedded in coderd. The same LLM request
// against the same provider must produce the same response and an equivalent
// interception record in both deployment modes, so operators can move traffic
// between them without behavior changes. Both legs use only public surface:
// the embedded gateway is reached through coderd's route and the standalone
// gateway runs as the real ai-gateway start command.
func TestGatewayParityE2E_EmbeddedVsStandalone(t *testing.T) {
	t.Parallel()

	embeddedMode := gatewayMode{
		name: "embedded",
		endpoint: func(ctx context.Context, t *testing.T, dep *e2eDeployment) string {
			aibridgedtest.StartTestAIBridgeDaemon(ctx, t, dep.api.AGPL, nil)
			return dep.client.URL.JoinPath("/api/v2/ai-gateway/openai/v1/chat/completions").String()
		},
	}
	standaloneMode := gatewayMode{
		name: "standalone",
		endpoint: func(ctx context.Context, t *testing.T, dep *e2eDeployment) string {
			baseURL, _ := startAIGatewayCommand(ctx, t, dep.client.URL.String(), dep.key)
			return baseURL + "/openai/v1/chat/completions"
		},
	}

	// The modes run in the same test rather than as parallel subtests so their
	// outcomes can be compared against each other.
	ctx := testutil.Context(t, testutil.WaitLong)
	embedded := serveThroughGateway(ctx, t, embeddedMode)
	standalone := serveThroughGateway(ctx, t, standaloneMode)

	require.Equal(t, embedded.status, standalone.status)
	require.Equal(t, normalizedCompletionBody(t, embedded.body), normalizedCompletionBody(t, standalone.body))
	// IDs and timestamps are deployment-specific, so parity is asserted on the
	// attribution fields a consumer of the audit trail reads.
	require.Equal(t, embedded.session.Providers, standalone.session.Providers)
	require.Equal(t, embedded.session.Models, standalone.session.Models)
	require.Equal(t, embedded.session.Client, standalone.session.Client)
	require.Equal(t, embedded.session.Threads, standalone.session.Threads)
}

// serveThroughGateway stands up a coderd, a provider, and the gateway for mode,
// then sends the shared LLM request through it. Each mode gets its own coderd,
// upstream, and initiating user.
func serveThroughGateway(ctx context.Context, t *testing.T, mode gatewayMode) gatewayOutcome {
	t.Helper()

	dep := setupE2EDeployment(ctx, t)
	endpoint := mode.endpoint(ctx, t, dep)

	// The gateway loads providers asynchronously, so the route may not be
	// wired up on the first attempt.
	var (
		status int
		body   []byte
	)
	require.Eventuallyf(t, func() bool {
		var err error
		status, body, err = postParityChatCompletion(ctx, endpoint, dep.userClient.SessionToken())
		return err == nil && status == http.StatusOK
	}, testutil.WaitLong, testutil.IntervalFast,
		"%s gateway never served a chat completion: status=%d, body=%s", mode.name, status, body)

	require.Containsf(t, string(body), "standalone gateway e2e response",
		"%s gateway must return the upstream response, body: %s", mode.name, body)
	require.Equalf(t, int32(1), dep.upstreamHits.Load(),
		"%s gateway must forward exactly one request upstream", mode.name)

	var sessions []codersdk.AIBridgeSession
	require.Eventuallyf(t, func() bool {
		//nolint:gocritic // Owner role is needed to list every user's sessions.
		resp, err := dep.client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Initiator: dep.user.Username,
		})
		if err != nil {
			return false
		}
		sessions = resp.Sessions
		return len(sessions) == 1
	}, testutil.WaitLong, testutil.IntervalFast,
		"%s gateway must record exactly one interception, got %d", mode.name, len(sessions))
	require.Equalf(t, dep.user.Username, sessions[0].Initiator.Username,
		"%s gateway must attribute the interception to the initiating user", mode.name)

	return gatewayOutcome{status: status, body: body, session: sessions[0]}
}

// postParityChatCompletion sends the shared LLM request to endpoint and
// reports transport errors instead of failing the test, so callers can retry.
func postParityChatCompletion(ctx context.Context, endpoint, token string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(e2eChatCompletionRequest))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, body, err
}

// normalizedCompletionBody decodes a chat completion and drops the id, which
// both modes replace with their own per-request interception ID.
func normalizedCompletionBody(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded), "body: %s", body)
	id, ok := decoded["id"].(string)
	require.True(t, ok, "response must carry an id: %s", body)
	_, err := uuid.Parse(id)
	require.NoError(t, err, "the completion id must be the interception ID, got %q", id)
	delete(decoded, "id")
	return decoded
}
