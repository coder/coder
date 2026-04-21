package integrationtest

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"cdr.dev/slog/v3"
	"github.com/coder/aibridge"
	"github.com/coder/aibridge/config"
	aibcontext "github.com/coder/aibridge/context"
	"github.com/coder/aibridge/fixtures"
	"github.com/coder/aibridge/internal/testutil"
	"github.com/coder/aibridge/mcp"
	"github.com/coder/aibridge/metrics"
	"github.com/coder/aibridge/provider"
	"github.com/coder/aibridge/recorder"
)

const (
	pathAnthropicMessages      = "/anthropic/v1/messages"
	pathOpenAIChatCompletions  = "/openai/v1/chat/completions"
	pathOpenAIResponses        = "/openai/v1/responses"
	pathCopilotChatCompletions = "/copilot/chat/completions"
	pathCopilotResponses       = "/copilot/responses"

	// providerBedrock identifies a Bedrock provider in [withProvider].
	// other providers use config.Provider* constants.
	providerBedrock = "bedrock"

	// defaults
	apiKey         = "api-key"
	defaultActorID = "ae235cc1-9f8f-417d-a636-a7b170bac62e"
)

var defaultTracer = otel.Tracer("integrationtest")

type bridgeConfig struct {
	providerBuilders []func(upstreamURL string) aibridge.Provider
	metrics          *metrics.Metrics
	tracer           trace.Tracer
	mcpProxy         mcp.ServerProxier
	userID           string
	metadata         recorder.Metadata
	logger           slog.Logger
}

// bridgeTestServer wraps an httptest.Server running a RequestBridge.
type bridgeTestServer struct {
	*httptest.Server
	Recorder *testutil.MockRecorder
	Bridge   *aibridge.RequestBridge
}

// makeRequest builds and executes an HTTP request against this server.
// Optional headers are applied after the default Content-Type.
func (s *bridgeTestServer) makeRequest(t *testing.T, method string, path string, body []byte, header ...http.Header) (*http.Response, error) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, s.URL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for _, h := range header {
		for k, vals := range h {
			for _, v := range vals {
				req.Header.Add(k, v)
			}
		}
	}
	return http.DefaultClient.Do(req)
}

type bridgeOption func(*bridgeConfig)

// withProvider adds a default-configured provider of the given type.
// When any provider option is used, the default "all providers" set is not created.
func withProvider(providerType string) bridgeOption {
	return func(c *bridgeConfig) {
		c.providerBuilders = append(c.providerBuilders, func(addr string) aibridge.Provider {
			return newDefaultProvider(providerType, addr)
		})
	}
}

// withCustomProvider adds a pre-built provider. The upstream URL passed to
// [newBridgeTestServer] is ignored for this provider.
// When any provider option is used, the default "all providers" set is not created.
func withCustomProvider(p aibridge.Provider) bridgeOption {
	return func(c *bridgeConfig) {
		c.providerBuilders = append(c.providerBuilders, func(string) aibridge.Provider {
			return p
		})
	}
}

// withMetrics sets the Prometheus metrics for the bridge.
func withMetrics(m *metrics.Metrics) bridgeOption {
	return func(c *bridgeConfig) { c.metrics = m }
}

// withTracer overrides the default tracer.
func withTracer(t trace.Tracer) bridgeOption {
	return func(c *bridgeConfig) { c.tracer = t }
}

// withMCP sets the MCP server proxier (default: NoopMCPManager).
func withMCP(p mcp.ServerProxier) bridgeOption {
	return func(c *bridgeConfig) { c.mcpProxy = p }
}

// withActor sets the actor ID and metadata for the BaseContext.
func withActor(id string, md recorder.Metadata) bridgeOption {
	return func(c *bridgeConfig) { c.userID = id; c.metadata = md }
}

// newBridgeTestServer creates a fully configured test server running
// a RequestBridge with sensible defaults:
//   - All standard providers (unless withProvider / withCustomProvider)
//   - NoopMCPManager (unless withMCP)
//   - slogtest debug logger
//   - defaultTracer (unless withTracer)
//   - defaultActorID (unless withActor)
func newBridgeTestServer(
	ctx context.Context,
	t *testing.T,
	upstreamURL string,
	opts ...bridgeOption,
) *bridgeTestServer {
	t.Helper()

	cfg := &bridgeConfig{
		userID: defaultActorID,
	}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.tracer == nil {
		cfg.tracer = defaultTracer
	}
	cfg.logger = newLogger(t)
	if cfg.mcpProxy == nil {
		cfg.mcpProxy = newNoopMCPManager()
	}

	// Resolve providers: use explicit builders when provided, otherwise
	// create default providers for every supported type.
	var providers []aibridge.Provider
	if len(cfg.providerBuilders) > 0 {
		for _, b := range cfg.providerBuilders {
			providers = append(providers, b(upstreamURL))
		}
	} else {
		providers = []aibridge.Provider{
			newDefaultProvider(config.ProviderAnthropic, upstreamURL),
			newDefaultProvider(config.ProviderOpenAI, upstreamURL),
		}
	}

	mockRec := &testutil.MockRecorder{}
	rec := aibridge.NewRecorder(cfg.logger, cfg.tracer, func() (aibridge.Recorder, error) {
		return mockRec, nil
	})

	bridge, err := aibridge.NewRequestBridge(
		ctx, providers, rec, cfg.mcpProxy,
		cfg.logger, cfg.metrics, cfg.tracer,
	)
	require.NoError(t, err)

	actorID, md := cfg.userID, cfg.metadata
	srv := httptest.NewUnstartedServer(bridge)
	srv.Config.BaseContext = func(_ net.Listener) context.Context {
		return aibcontext.AsActor(ctx, actorID, md)
	}
	srv.Start()
	t.Cleanup(srv.Close)

	return &bridgeTestServer{
		Server:   srv,
		Recorder: mockRec,
		Bridge:   bridge,
	}
}

// setupInjectedToolTest abstracts common setup required for injected-tool integration tests.
// Extra bridge options (e.g. [withProvider]) are appended after the built-in
// MCP / tracer / actor options. When no provider option is given the default
// provider set (all providers) is used.
func setupInjectedToolTest(
	t *testing.T,
	fixture []byte,
	streaming bool,
	tracer trace.Tracer,
	path string,
	toolRequestValidatorFn func(*http.Request, []byte),
	opts ...bridgeOption,
) (*bridgeTestServer, *mockMCP, *http.Response) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	t.Cleanup(cancel)

	fix := fixtures.Parse(t, fixture)

	// Setup mock server for multi-turn interaction.
	// First request → tool call response
	// Second request → final response.
	firstResp := newFixtureResponse(fix)
	toolResp := newFixtureToolResponse(fix)
	toolResp.OnRequest = toolRequestValidatorFn
	upstream := newMockUpstream(ctx, t, firstResp, toolResp)

	mockMCP := setupMCPForTest(t, tracer)

	allOpts := []bridgeOption{
		withMCP(mockMCP),
		withTracer(tracer),
		withActor(defaultActorID, nil),
	}
	allOpts = append(allOpts, opts...)
	bridgeServer := newBridgeTestServer(ctx, t, upstream.URL, allOpts...)

	// Add the stream param to the request.
	reqBody, err := sjson.SetBytes(fix.Request(), "stream", streaming)
	require.NoError(t, err)

	resp, err := bridgeServer.makeRequest(t, http.MethodPost, path, reqBody)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Wait both requests (initial + tool call result)
	require.Eventually(t, func() bool {
		return upstream.Calls.Load() == 2
	}, testutil.WaitMedium, testutil.IntervalFast)

	return bridgeServer, mockMCP, resp
}

// newDefaultProvider creates a Provider with default test configuration.
func newDefaultProvider(providerType string, addr string) aibridge.Provider {
	switch providerType {
	case config.ProviderAnthropic:
		return provider.NewAnthropic(anthropicCfg(addr, apiKey), nil)
	case config.ProviderOpenAI:
		return provider.NewOpenAI(openAICfg(addr, apiKey))
	case providerBedrock:
		return provider.NewAnthropic(anthropicCfg(addr, apiKey), bedrockCfg(addr))
	default:
		panic("unknown provider type: " + providerType)
	}
}
