package coderd_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
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
// This mirrors what cli/server.go does in production so /api/v2/ai-gateway
// requests dispatch through the real pool and reloader.
func startTestAIBridgeDaemon(t *testing.T, api *coderd.API) *aibridged.Metrics {
	t.Helper()

	ctx := context.Background()
	logger := slogtest.Make(t, nil).Named("aibridged").Leveled(slog.LevelDebug)
	cfg := api.DeploymentValues.AI.BridgeConfig
	tracer := otel.Tracer("aibridge-reload-test")

	providers, _, err := cli.BuildProviders(ctx, api.Database, cfg, logger, nil)
	require.NoError(t, err)

	pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, providers, logger.Named("pool"), nil, tracer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	metrics := aibridged.NewMetrics(prometheus.NewRegistry())
	reloader := &testPoolReloader{pool: pool, db: api.Database, cfg: cfg, logger: logger.Named("reloader"), metrics: metrics}
	unsubscribe, err := aibridged.SubscribeProviderReload(ctx, api.Pubsub, reloader, logger.Named("subscriber"))
	require.NoError(t, err)
	t.Cleanup(unsubscribe)

	srv, err := aibridged.New(ctx, pool, func(dialCtx context.Context) (aibridged.DRPCClient, error) {
		return api.CreateInMemoryAIBridgeServer(dialCtx)
	}, logger, tracer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	api.RegisterInMemoryAIBridgedHTTPHandler(srv)
	return metrics
}

type testPoolReloader struct {
	pool    *aibridged.CachedBridgePool
	db      database.Store
	cfg     codersdk.AIBridgeConfig
	logger  slog.Logger
	metrics *aibridged.Metrics
}

func (r *testPoolReloader) Reload(ctx context.Context) error {
	defer r.metrics.RecordReloadAttempt()
	providers, outcomes, err := cli.BuildProviders(ctx, r.db, r.cfg, r.logger, nil)
	if err != nil {
		return err
	}
	r.pool.ReplaceProviders(providers)
	r.metrics.RecordReloadSuccess(outcomes)
	return nil
}

// TestAIBridgeProviderHotReload exercises the end-to-end CRUD ->
// reload -> routing path: every provider mutation made through codersdk
// must, within a short window, change the routing observed at
// /api/v2/ai-gateway/{name}/v1/models. The OpenAI passthrough route
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

	metrics := startTestAIBridgeDaemon(t, api.AGPL)

	// requireProviderStatus polls until the provider_info series for
	// (name, status) settles to value 1. Reloads happen via pubsub, so
	// the assertion has to be eventual.
	requireProviderStatus := func(t *testing.T, name, status string) {
		t.Helper()
		require.Eventuallyf(t, func() bool {
			return promtest.ToFloat64(metrics.ProviderInfo.WithLabelValues(name, "openai", status)) == 1
		}, testutil.WaitShort, testutil.IntervalFast,
			"expected provider_info{provider_name=%q, status=%q} == 1", name, status)
	}

	// requireProviderAbsent polls until no series exists for the
	// provider name in any status. After a delete the Reset on the
	// next reload must clear all previous status labels for the name.
	requireProviderAbsent := func(t *testing.T, name string) {
		t.Helper()
		require.Eventuallyf(t, func() bool {
			for _, status := range []string{"enabled", "disabled", "error"} {
				if promtest.ToFloat64(metrics.ProviderInfo.WithLabelValues(name, "openai", status)) != 0 {
					return false
				}
			}
			return true
		}, testutil.WaitShort, testutil.IntervalFast,
			"expected provider_info series for %q to be cleared after delete", name)
	}

	ctx := testutil.Context(t, testutil.WaitLong)

	// sendRequest issues GET /api/v2/ai-gateway/{name}/v1/models and
	// returns the status and the upstream marker decoded from the
	// JSON body (empty if the body was not the marker JSON).
	sendRequest := func(providerName string) (int, string) {
		url := client.URL.String() + "/api/v2/ai-gateway/" + providerName + "/v1/models"
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

	// requireDisabledSentinel polls until the provider name yields a
	// 503 with the provider_disabled body, indicating the disabled
	// handler is wired up for the row.
	requireDisabledSentinel := func(t *testing.T, providerName string) {
		t.Helper()
		require.Eventuallyf(t, func() bool {
			status, _ := sendRequest(providerName)
			return status == http.StatusServiceUnavailable
		}, testutil.WaitShort, testutil.IntervalFast,
			"expected provider %q to serve the disabled sentinel", providerName)
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
	requireProviderStatus(t, "primary", "enabled")

	// 2. Update BaseURL: same name, now points at upstream B.
	newBaseURL := upstreamB.server.URL
	_, err = client.UpdateAIProvider(ctx, "primary", codersdk.UpdateAIProviderRequest{
		BaseURL: &newBaseURL,
	})
	require.NoError(t, err)
	requireRoutesTo(t, "primary", upstreamB)
	requireProviderStatus(t, "primary", "enabled")

	// 3. Disable: requests stop reaching upstream and the bridge
	// answers with the 503 sentinel. The metric flips to "disabled".
	disabled := false
	_, err = client.UpdateAIProvider(ctx, "primary", codersdk.UpdateAIProviderRequest{
		Enabled: &disabled,
	})
	require.NoError(t, err)
	requireDisabledSentinel(t, "primary")
	requireProviderStatus(t, "primary", "disabled")

	// 4. Re-enable: routing comes back at the most recent BaseURL.
	enabled := true
	_, err = client.UpdateAIProvider(ctx, "primary", codersdk.UpdateAIProviderRequest{
		Enabled: &enabled,
	})
	require.NoError(t, err)
	requireRoutesTo(t, "primary", upstreamB)
	requireProviderStatus(t, "primary", "enabled")

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
	requireProviderStatus(t, "primary", "enabled")
	requireProviderStatus(t, "secondary", "enabled")

	// 6. Delete primary: only secondary remains routable. The
	// provider_info series for primary disappears entirely on the
	// next reload's Reset.
	require.NoError(t, client.DeleteAIProvider(ctx, "primary"))
	requireRoutingGone(t, "primary")
	requireRoutesTo(t, "secondary", upstreamA)
	requireProviderAbsent(t, "primary")
	requireProviderStatus(t, "secondary", "enabled")

	// Both timestamp gauges must have advanced during this test.
	assert.Positive(t, promtest.ToFloat64(metrics.ProvidersLastReloadTimestampSeconds))
	assert.Positive(t, promtest.ToFloat64(metrics.ProvidersLastReloadSuccessTimestampSeconds))
}
