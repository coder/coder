package coderd_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
	"github.com/coder/websocket"
)

const (
	wsGatewayCreate    = `{"type":"response.create","model":"gpt-5","input":[{"role":"user","content":[{"type":"input_text","text":"why is the sky blue?"}]}]}`
	wsGatewayCreated   = `{"type":"response.created","response":{"id":"resp_embedded","model":"gpt-5","status":"in_progress"}}`
	wsGatewayCompleted = `{"type":"response.completed","response":{"id":"resp_embedded","model":"gpt-5","status":"completed","usage":{"input_tokens":60,"output_tokens":15,"total_tokens":75}}}`
)

// wsGatewayDeployment is a coderd with the embedded AI Gateway whose OpenAI
// provider points at an upstream that accepts Responses WebSockets.
type wsGatewayDeployment struct {
	db         database.Store
	userClient *codersdk.Client
	// upstreams receives each accepted upstream connection and its
	// handshake headers.
	upstreams chan *websocket.Conn
	headers   chan http.Header
}

// startWSGateway starts the deployment with the given experiments on.
func startWSGateway(ctx context.Context, t *testing.T, experiments ...codersdk.Experiment) *wsGatewayDeployment {
	t.Helper()
	d := &wsGatewayDeployment{upstreams: make(chan *websocket.Conn, 4), headers: make(chan http.Header, 4)}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			// Ordinary passthrough requests succeed.
			w.WriteHeader(http.StatusOK)
			return
		}
		d.headers <- r.Header.Clone()
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("upstream accept: %v", err)
			return
		}
		d.upstreams <- conn
	}))
	t.Cleanup(upstream.Close)

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	// An open socket must not hold the only concurrency slot.
	dv.AI.BridgeConfig.MaxConcurrency = serpent.Int64(1)
	for _, ex := range experiments {
		dv.Experiments = append(dv.Experiments, string(ex))
	}
	db, ps := dbtestutil.NewDB(t)
	client, _, api, firstUser := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv, Database: db, Pubsub: ps},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureAIBridge: 1},
		},
	})
	//nolint:gocritic // Owner role is needed for provider management.
	_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
		Type:    codersdk.AIProviderTypeOpenAI,
		Name:    "openai",
		Enabled: true,
		BaseURL: upstream.URL,
		APIKeys: []string{"sk-embedded"},
	})
	require.NoError(t, err)
	aibridgedtest.StartTestAIBridgeDaemon(ctx, t, api.AGPL, nil)
	d.db = db
	d.userClient, _ = coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
	return d
}

func (d *wsGatewayDeployment) url(path string) string {
	return d.userClient.URL.String() + "/api/v2/ai-gateway/openai/v1" + path
}

func (d *wsGatewayDeployment) dial(ctx context.Context, path string) (*websocket.Conn, *http.Response, error) {
	//nolint:bodyclose // Dial owns the response body.
	return websocket.Dial(ctx, "ws"+strings.TrimPrefix(d.url(path), "http"), &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + d.userClient.SessionToken()}},
	})
}

// TestEmbeddedAIGatewayResponsesWebSocket requires that a user with the
// experiment on gets Responses WebSocket mode through /api/v2/ai-gateway:
// the socket reaches upstream with only the provider key, frees its
// concurrency slot while open, and its create is recorded like an HTTP
// request. Passthrough routes refuse upgrades.
func TestEmbeddedAIGatewayResponsesWebSocket(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	d := startWSGateway(ctx, t, codersdk.ExperimentAIGatewayResponsesWebSocket)

	//nolint:bodyclose // Dial owns the response body.
	client, resp, err := d.dial(ctx, "/responses")
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	defer client.CloseNow()
	upstream := testutil.TryReceive(ctx, t, d.upstreams)
	defer upstream.CloseNow()
	headers := testutil.TryReceive(ctx, t, d.headers)
	assert.Equal(t, []string{"Bearer sk-embedded"}, headers.Values("Authorization"))

	require.NoError(t, client.Write(ctx, websocket.MessageText, []byte(wsGatewayCreate)))
	_, msg, err := upstream.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, wsGatewayCreate, string(msg))
	for _, frame := range []string{wsGatewayCreated, wsGatewayCompleted} {
		require.NoError(t, upstream.Write(ctx, websocket.MessageText, []byte(frame)))
		_, msg, err := client.Read(ctx)
		require.NoError(t, err)
		require.Equal(t, frame, string(msg))
	}

	// The open socket does not hold the only concurrency slot.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.url("/models"), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+d.userClient.SessionToken())
	httpResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = httpResp.Body.Close()
	assert.Equal(t, http.StatusOK, httpResp.StatusCode)

	// Passthrough routes never tunnel an upgrade.
	//nolint:bodyclose // Dial owns the response body.
	_, resp, err = d.dial(ctx, "/models")
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)

	var interceptions []database.AIBridgeInterception
	require.Eventually(t, func() bool {
		interceptions, err = d.db.GetAIBridgeInterceptions(ctx)
		return err == nil && len(interceptions) == 1 && interceptions[0].EndedAt.Valid
	}, testutil.WaitLong, testutil.IntervalFast, "the create should be recorded and ended")
	assert.Equal(t, "gpt-5", interceptions[0].Model)
	prompts, err := d.db.GetAIBridgeUserPromptsByInterceptionID(ctx, interceptions[0].ID)
	require.NoError(t, err)
	require.Len(t, prompts, 1)
	assert.Equal(t, "why is the sky blue?", prompts[0].Prompt)
	tokens, err := d.db.GetAIBridgeTokenUsagesByInterceptionID(ctx, interceptions[0].ID)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	assert.EqualValues(t, 15, tokens[0].OutputTokens)
}

// TestEmbeddedAIGatewayResponsesWebSocketExperimentOff requires that users
// without the experiment keep today's behavior: the upgrade is refused, so
// clients fall back to HTTP.
func TestEmbeddedAIGatewayResponsesWebSocketExperimentOff(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	d := startWSGateway(ctx, t)

	//nolint:bodyclose // Dial owns the response body.
	_, resp, err := d.dial(ctx, "/responses")
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
	assert.Empty(t, d.headers)
}
