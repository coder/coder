package aibridgeproxyd

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/testutil"
)

// TestReloadUpdatesProviderMetrics covers the provider_info GaugeVec
// surface: every reload pass rewrites the series for the current
// snapshot, including disabled and errored rows; the Reset on each
// reload drops series for providers that have left the configuration.
func TestReloadUpdatesProviderMetrics(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)

	reload := ProviderReload{Providers: []ReloadedProvider{
		{ProviderOutcome: aibridged.ProviderOutcome{Name: "alpha", Type: "openai", Status: aibridged.ProviderStatusEnabled}, Host: "alpha.example.com"},
		{ProviderOutcome: aibridged.ProviderOutcome{Name: "beta", Type: "anthropic", Status: aibridged.ProviderStatusDisabled}},
		{ProviderOutcome: aibridged.ProviderOutcome{Name: "gamma", Type: "openai", Status: aibridged.ProviderStatusError, Err: xerrors.New("bad config")}},
	}}

	ctx := testutil.Context(t, testutil.WaitShort)
	srv := &Server{
		ctx:          ctx,
		logger:       slogtest.Make(t, nil),
		allowedPorts: []string{"443"},
		metrics:      metrics,
		refreshProviders: func(context.Context) (ProviderReload, error) {
			return reload, nil
		},
	}
	srv.providerRouter.Store(emptyProviderRouter)

	before := time.Now().Unix()
	require.NoError(t, srv.Reload(ctx))
	after := time.Now().Unix()

	assert.Equal(t, 1.0, promtest.ToFloat64(metrics.ProviderInfo.WithLabelValues("alpha", "openai", "enabled")))
	assert.Equal(t, 1.0, promtest.ToFloat64(metrics.ProviderInfo.WithLabelValues("beta", "anthropic", "disabled")))
	assert.Equal(t, 1.0, promtest.ToFloat64(metrics.ProviderInfo.WithLabelValues("gamma", "openai", "error")))

	attemptTS := int64(promtest.ToFloat64(metrics.ProvidersLastReloadTimestampSeconds))
	successTS := int64(promtest.ToFloat64(metrics.ProvidersLastReloadSuccessTimestampSeconds))
	assert.GreaterOrEqual(t, attemptTS, before)
	assert.LessOrEqual(t, attemptTS, after)
	assert.GreaterOrEqual(t, successTS, before)
	assert.LessOrEqual(t, successTS, after)
}

// TestReloadResetsStaleProviderSeries verifies that providers removed
// between reloads do not leave behind stale series. Without Reset, a
// removed provider's last-seen value would persist for 5+ minutes and
// could fire alerts despite the provider no longer being configured.
func TestReloadResetsStaleProviderSeries(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)

	current := ProviderReload{Providers: []ReloadedProvider{
		{ProviderOutcome: aibridged.ProviderOutcome{Name: "alpha", Type: "openai", Status: aibridged.ProviderStatusEnabled}, Host: "alpha.example.com"},
		{ProviderOutcome: aibridged.ProviderOutcome{Name: "beta", Type: "anthropic", Status: aibridged.ProviderStatusEnabled}, Host: "beta.example.com"},
	}}

	ctx := testutil.Context(t, testutil.WaitShort)
	srv := &Server{
		ctx:          ctx,
		logger:       slogtest.Make(t, nil),
		allowedPorts: []string{"443"},
		metrics:      metrics,
		refreshProviders: func(context.Context) (ProviderReload, error) {
			return current, nil
		},
	}
	srv.providerRouter.Store(emptyProviderRouter)

	require.NoError(t, srv.Reload(ctx))
	require.Equal(t, 2, promtest.CollectAndCount(metrics.ProviderInfo))

	current = ProviderReload{Providers: []ReloadedProvider{
		{ProviderOutcome: aibridged.ProviderOutcome{Name: "alpha", Type: "openai", Status: aibridged.ProviderStatusEnabled}, Host: "alpha.example.com"},
	}}
	require.NoError(t, srv.Reload(ctx))

	assert.Equal(t, 1, promtest.CollectAndCount(metrics.ProviderInfo),
		"beta should have been Reset out of the GaugeVec")
	assert.Equal(t, 1.0, promtest.ToFloat64(metrics.ProviderInfo.WithLabelValues("alpha", "openai", "enabled")))
}

// TestReloadAttemptTimestampUpdatesOnFailure asserts the attempt-time
// gauge advances even when the refresh function fails, while the
// success-time gauge does not.
func TestReloadAttemptTimestampUpdatesOnFailure(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)
	refreshErr := xerrors.New("simulated failure")

	ctx := testutil.Context(t, testutil.WaitShort)
	srv := &Server{
		ctx:          ctx,
		logger:       slogtest.Make(t, nil),
		allowedPorts: []string{"443"},
		metrics:      metrics,
		refreshProviders: func(context.Context) (ProviderReload, error) {
			return ProviderReload{}, refreshErr
		},
	}
	srv.providerRouter.Store(emptyProviderRouter)

	before := time.Now().Unix()
	err := srv.Reload(ctx)
	require.ErrorIs(t, err, refreshErr)
	after := time.Now().Unix()

	attemptTS := int64(promtest.ToFloat64(metrics.ProvidersLastReloadTimestampSeconds))
	successTS := int64(promtest.ToFloat64(metrics.ProvidersLastReloadSuccessTimestampSeconds))
	assert.GreaterOrEqual(t, attemptTS, before)
	assert.LessOrEqual(t, attemptTS, after)
	assert.Equal(t, int64(0), successTS, "success timestamp must not advance on failure")
}
