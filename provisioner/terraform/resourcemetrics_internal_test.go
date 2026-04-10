package terraform

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestResourceMetricsRegistration(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()
	m := newResourceMetrics(registry)
	require.NotNil(t, m.providerMemoryPeakBytes)
	require.NotNil(t, m.providerCPUSeconds)

	families, err := registry.Gather()
	require.NoError(t, err)

	names := make(map[string]bool, len(families))
	for _, f := range families {
		names[f.GetName()] = true
	}

	// GaugeVec with no observations still registers the family
	// name after a Set call, but Gather only returns families that
	// have been observed. Force an observation so Gather picks
	// them up.
	m.providerMemoryPeakBytes.WithLabelValues("test", "init").Set(0)
	m.providerCPUSeconds.WithLabelValues("test", "init").Set(0)

	families, err = registry.Gather()
	require.NoError(t, err)

	names = make(map[string]bool, len(families))
	for _, f := range families {
		names[f.GetName()] = true
	}

	require.True(t, names["coderd_provisionerd_provider_memory_peak_bytes"],
		"expected memory metric to be registered, got: %v", names)
	require.True(t, names["coderd_provisionerd_provider_cpu_seconds"],
		"expected cpu metric to be registered, got: %v", names)
}

func TestResourceMetricsObservation(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()
	m := newResourceMetrics(registry)

	sample := &ProcessSample{
		Providers: map[string]ProviderResourceUsage{
			"aws": {
				PeakRSSBytes:   1024 * 1024 * 256,
				CPUTimeSeconds: 4.2,
			},
			"google": {
				PeakRSSBytes:   1024 * 1024 * 128,
				CPUTimeSeconds: 1.5,
			},
		},
	}

	observeResourceMetrics(m, "plan", sample)

	families, err := registry.Gather()
	require.NoError(t, err)

	// Index gathered metrics for easy lookup.
	byName := make(map[string]*dto.MetricFamily, len(families))
	for _, f := range families {
		byName[f.GetName()] = f
	}

	memFamily := byName["coderd_provisionerd_provider_memory_peak_bytes"]
	require.NotNil(t, memFamily, "memory metric family not found")

	cpuFamily := byName["coderd_provisionerd_provider_cpu_seconds"]
	require.NotNil(t, cpuFamily, "cpu metric family not found")

	// Build a lookup of provider+stage -> gauge value.
	type key struct {
		provider, stage string
	}
	memValues := gatherGaugeValues(t, memFamily)
	cpuValues := gatherGaugeValues(t, cpuFamily)

	require.InDelta(t, float64(1024*1024*256), memValues[key{"aws", "plan"}], 0.1)
	require.InDelta(t, float64(1024*1024*128), memValues[key{"google", "plan"}], 0.1)
	require.InDelta(t, 4.2, cpuValues[key{"aws", "plan"}], 0.001)
	require.InDelta(t, 1.5, cpuValues[key{"google", "plan"}], 0.001)
}

func TestObserveResourceMetricsNilSafety(t *testing.T) {
	t.Parallel()

	// None of these should panic.
	observeResourceMetrics(nil, "plan", &ProcessSample{})
	observeResourceMetrics(nil, "plan", nil)

	registry := prometheus.NewRegistry()
	m := newResourceMetrics(registry)
	observeResourceMetrics(m, "plan", nil)
	observeResourceMetrics(m, "plan", &ProcessSample{})
	observeResourceMetrics(m, "plan", &ProcessSample{
		Providers: map[string]ProviderResourceUsage{},
	})
}

// gatherGaugeValues extracts provider+stage -> gauge value from a
// metric family.
func gatherGaugeValues(t *testing.T, family *dto.MetricFamily) map[struct{ provider, stage string }]float64 {
	t.Helper()
	type key = struct{ provider, stage string }
	result := make(map[key]float64)
	for _, metric := range family.GetMetric() {
		var provider, stage string
		for _, lp := range metric.GetLabel() {
			switch lp.GetName() {
			case "provider":
				provider = lp.GetValue()
			case "stage":
				stage = lp.GetValue()
			}
		}
		result[key{provider, stage}] = metric.GetGauge().GetValue()
	}
	return result
}
