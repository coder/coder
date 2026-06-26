package aibridged_test

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridged"
)

// TestMetricsRecordReloadSuccess covers the provider_info GaugeVec
// surface: every reload pass rewrites the series for the current
// outcomes and the Reset on each pass drops stale series.
func TestMetricsRecordReloadSuccess(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := aibridged.NewMetrics(reg)

	outcomes := []aibridged.ProviderOutcome{
		{Name: "alpha", Type: "openai", Status: aibridged.ProviderStatusEnabled},
		{Name: "beta", Type: "anthropic", Status: aibridged.ProviderStatusDisabled},
		{Name: "gamma", Type: "openai", Status: aibridged.ProviderStatusError, Err: xerrors.New("bad config")},
	}

	before := time.Now().Unix()
	m.RecordReloadAttempt()
	m.RecordReloadSuccess(outcomes)
	after := time.Now().Unix()

	assert.Equal(t, 1.0, promtest.ToFloat64(m.ProviderInfo.WithLabelValues("alpha", "openai", "enabled")))
	assert.Equal(t, 1.0, promtest.ToFloat64(m.ProviderInfo.WithLabelValues("beta", "anthropic", "disabled")))
	assert.Equal(t, 1.0, promtest.ToFloat64(m.ProviderInfo.WithLabelValues("gamma", "openai", "error")))

	attemptTS := int64(promtest.ToFloat64(m.ProvidersLastReloadTimestampSeconds))
	successTS := int64(promtest.ToFloat64(m.ProvidersLastReloadSuccessTimestampSeconds))
	assert.GreaterOrEqual(t, attemptTS, before)
	assert.LessOrEqual(t, attemptTS, after)
	assert.GreaterOrEqual(t, successTS, before)
	assert.LessOrEqual(t, successTS, after)
}

// TestMetricsResetsStaleProviderSeries verifies that providers removed
// from the outcome set between reloads do not leave behind stale
// series.
func TestMetricsResetsStaleProviderSeries(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := aibridged.NewMetrics(reg)

	m.RecordReloadSuccess([]aibridged.ProviderOutcome{
		{Name: "alpha", Type: "openai", Status: aibridged.ProviderStatusEnabled},
		{Name: "beta", Type: "anthropic", Status: aibridged.ProviderStatusEnabled},
	})
	require.Equal(t, 2, promtest.CollectAndCount(m.ProviderInfo))

	m.RecordReloadSuccess([]aibridged.ProviderOutcome{
		{Name: "alpha", Type: "openai", Status: aibridged.ProviderStatusEnabled},
	})

	assert.Equal(t, 1, promtest.CollectAndCount(m.ProviderInfo),
		"beta should have been Reset out of the GaugeVec")
	assert.Equal(t, 1.0, promtest.ToFloat64(m.ProviderInfo.WithLabelValues("alpha", "openai", "enabled")))
}

// TestMetricsNilSafe asserts the helpers tolerate a nil receiver so
// callers can pass `nil` to disable metric updates without guarding
// every call site.
func TestMetricsNilSafe(t *testing.T) {
	t.Parallel()

	var m *aibridged.Metrics
	require.NotPanics(t, func() {
		m.RecordReloadAttempt()
		m.RecordReloadSuccess(nil)
		m.Unregister()
	})
}
