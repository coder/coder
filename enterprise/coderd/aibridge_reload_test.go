package coderd_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// mockUpstream is a single httptest server identified by a unique
// marker that it echoes in every response body, so callers can verify
// which upstream a proxied request actually reached. The hit counter
// supports asserting the upstream was touched at all.
type mockUpstream struct {
	server *httptest.Server
	name   string
	hits   atomic.Int32
}

func newMockUpstream(t *testing.T, name string) *mockUpstream {
	t.Helper()
	m := &mockUpstream{name: name}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		m.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]string{"upstream": name}))
	}))
	t.Cleanup(m.server.Close)
	return m
}

// startTestAIBridgeDaemon wires an in-process aibridged daemon onto
// the supplied API and subscribes it to ai_providers change events.
// This mirrors what cli/server.go does in production so /api/v2/aibridge
// requests dispatch through the real pool and reloader.
func startTestAIBridgeDaemon(t *testing.T, api *coderd.API) {
	t.Helper()

	ctx := context.Background()
	logger := slogtest.Make(t, nil).Named("aibridged").Leveled(slog.LevelDebug)
	cfg := api.DeploymentValues.AI.BridgeConfig
	tracer := otel.Tracer("aibridge-reload-test")

	providers, err := cli.BuildProviders(ctx, api.Database, cfg, logger)
	require.NoError(t, err)

	pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, providers, logger.Named("pool"), nil, tracer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	reloader := &testPoolReloader{pool: pool, db: api.Database, cfg: cfg, logger: logger.Named("reloader")}
	unsubscribe, err := aibridged.SubscribeProviderReload(ctx, api.Pubsub, reloader, logger.Named("subscriber"))
	require.NoError(t, err)
	t.Cleanup(unsubscribe)

	srv, err := aibridged.New(ctx, pool, func(dialCtx context.Context) (aibridged.DRPCClient, error) {
		return api.CreateInMemoryAIBridgeServer(dialCtx)
	}, logger, tracer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	api.RegisterInMemoryAIBridgedHTTPHandler(srv)
}

type testPoolReloader struct {
	pool   *aibridged.CachedBridgePool
	db     database.Store
	cfg    codersdk.AIBridgeConfig
	logger slog.Logger
}

func (r *testPoolReloader) Reload(ctx context.Context) error {
	providers, err := cli.BuildProviders(ctx, r.db, r.cfg, r.logger)
	if err != nil {
		return err
	}
	r.pool.ReplaceProviders(providers)
	return nil
}

// TestAIBridgeProviderHotReload exercises the end-to-end CRUD ->
// reload -> routing path: every provider mutation made through codersdk
// must, within a short window, change the routing observed at
// /api/v2/aibridge/{name}/v1/models. The OpenAI passthrough route
// /v1/models reverse-proxies to BaseURL, so the upstream that responds
// identifies which provider the daemon's mux dispatched to.
func TestAIBridgeProviderHotReload(t *testing.T) {
	t.Parallel()

	// Two distinct upstreams so an Update that swings the BaseURL is
	// observable: which upstream answers tells us which BaseURL the
	// freshly-built provider is pointed at.
	upstreamA := newMockUpstream(t, "a")
	upstreamB := newMockUpstream(t, "b")

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)

	client, _, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureAIBridge: 1},
		},
	})

	startTestAIBridgeDaemon(t, api.AGPL)

	ctx := testutil.Context(t, testutil.WaitLong)

	// sendRequest issues GET /api/v2/aibridge/{name}/v1/models and
	// returns the status and the upstream marker decoded from the
	// JSON body (empty if the body was not the marker JSON).
	sendRequest := func(providerName string) (int, string) {
		url := client.URL.String() + "/api/v2/aibridge/" + providerName + "/v1/models"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+client.SessionToken())

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return resp.StatusCode, ""
		}
		var decoded map[string]string
		_ = json.Unmarshal(body, &decoded)
		return resp.StatusCode, decoded["upstream"]
	}

	// requireRoutesTo polls until the routing reflects the expected
	// upstream. The pool reloads asynchronously from a pubsub event;
	// require.Eventually is the natural fit.
	requireRoutesTo := func(t *testing.T, providerName string, upstream *mockUpstream) {
		t.Helper()
		before := upstream.hits.Load()
		require.Eventuallyf(t, func() bool {
			status, marker := sendRequest(providerName)
			return status == http.StatusOK && marker == upstream.name
		}, testutil.WaitShort, testutil.IntervalFast,
			"expected provider %q to route to upstream %q", providerName, upstream.name)
		require.Greater(t, upstream.hits.Load(), before,
			"upstream %q must have observed at least one request", upstream.name)
	}

	// requireRoutingGone polls until the provider name yields a 404
	// from the aibridge mux's catch-all, indicating the provider has
	// been removed from the pool snapshot.
	requireRoutingGone := func(t *testing.T, providerName string) {
		t.Helper()
		require.Eventuallyf(t, func() bool {
			status, _ := sendRequest(providerName)
			return status == http.StatusNotFound
		}, testutil.WaitShort, testutil.IntervalFast,
			"expected provider %q to stop routing", providerName)
	}

	// 1. Create: provider points at upstream A.
	created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
		Type:    codersdk.AIProviderTypeOpenAI,
		Name:    "primary",
		Enabled: true,
		BaseURL: upstreamA.server.URL,
		APIKeys: []string{"sk-primary-key"},
	})
	require.NoError(t, err)
	require.Equal(t, "primary", created.Name)
	requireRoutesTo(t, "primary", upstreamA)

	// 2. Update BaseURL: same name, now points at upstream B.
	newBaseURL := upstreamB.server.URL
	_, err = client.UpdateAIProvider(ctx, "primary", codersdk.UpdateAIProviderRequest{
		BaseURL: &newBaseURL,
	})
	require.NoError(t, err)
	requireRoutesTo(t, "primary", upstreamB)

	// 3. Disable: the provider drops out of the snapshot, requests
	// stop reaching any upstream.
	disabled := false
	_, err = client.UpdateAIProvider(ctx, "primary", codersdk.UpdateAIProviderRequest{
		Enabled: &disabled,
	})
	require.NoError(t, err)
	requireRoutingGone(t, "primary")

	// 4. Re-enable: routing comes back at the most recent BaseURL.
	enabled := true
	_, err = client.UpdateAIProvider(ctx, "primary", codersdk.UpdateAIProviderRequest{
		Enabled: &enabled,
	})
	require.NoError(t, err)
	requireRoutesTo(t, "primary", upstreamB)

	// 5. Add a second provider; both names must route independently.
	_, err = client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
		Type:    codersdk.AIProviderTypeOpenAI,
		Name:    "secondary",
		Enabled: true,
		BaseURL: upstreamA.server.URL,
		APIKeys: []string{"sk-secondary-key"},
	})
	require.NoError(t, err)
	requireRoutesTo(t, "primary", upstreamB)
	requireRoutesTo(t, "secondary", upstreamA)

	// 6. Delete primary: only secondary remains routable.
	require.NoError(t, client.DeleteAIProvider(ctx, "primary"))
	requireRoutingGone(t, "primary")
	requireRoutesTo(t, "secondary", upstreamA)
}
