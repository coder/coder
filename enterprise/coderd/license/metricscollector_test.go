package license_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/aws/smithy-go/ptr"
	"github.com/prometheus/client_golang/prometheus"
	prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/license"
)

func TestCollectLicenseMetrics(t *testing.T) {
	t.Parallel()

	// Given
	registry := prometheus.NewRegistry()

	var sut license.MetricsCollector

	const (
		actualUsers = 4
		userLimit   = 7
	)
	sut.Entitlements = entitlements.New()
	sut.Entitlements.Modify(func(entitlements *codersdk.Entitlements) {
		entitlements.Features[codersdk.FeatureUserLimit] = codersdk.Feature{
			Enabled: true,
			Actual:  ptr.Int64(actualUsers),
			Limit:   ptr.Int64(userLimit),
		}
	})

	registry.Register(&sut)

	// When
	metrics, err := registry.Gather()
	require.NoError(t, err)

	// Then
	goldenFile, err := os.ReadFile("testdata/license-metrics.json")
	require.NoError(t, err)
	golden := map[string]int{}
	err = json.Unmarshal(goldenFile, &golden)
	require.NoError(t, err)

	for name, expected := range golden {
		actual, ok := findMetric(metrics, name)
		require.True(t, ok, "metric %s not found", name)
		require.Equal(t, expected, actual, "metric %s", name)
	}
}

func TestCollectLicenseMetrics_WarningsAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("NoWarningsOrErrors", func(t *testing.T) {
		t.Parallel()

		registry := prometheus.NewRegistry()
		var sut license.MetricsCollector
		sut.Entitlements = entitlements.New()

		registry.Register(&sut)

		metrics, err := registry.Gather()
		require.NoError(t, err)

		warnings, ok := findMetric(metrics, "coderd_license_warnings")
		require.True(t, ok)
		require.Zero(t, warnings)

		errors, ok := findMetric(metrics, "coderd_license_errors")
		require.True(t, ok)
		require.Zero(t, errors)
	})

	t.Run("WithWarnings", func(t *testing.T) {
		t.Parallel()

		registry := prometheus.NewRegistry()
		var sut license.MetricsCollector
		sut.Entitlements = entitlements.New()
		sut.Entitlements.Modify(func(entitlements *codersdk.Entitlements) {
			entitlements.Warnings = []string{
				"License expires in 30 days",
				"User limit is at 90% capacity",
			}
		})

		registry.Register(&sut)

		metrics, err := registry.Gather()
		require.NoError(t, err)

		warnings, ok := findMetric(metrics, "coderd_license_warnings")
		require.True(t, ok)
		require.Equal(t, 2, warnings)

		errors, ok := findMetric(metrics, "coderd_license_errors")
		require.True(t, ok)
		require.Zero(t, errors)
	})

	t.Run("WithErrors", func(t *testing.T) {
		t.Parallel()

		registry := prometheus.NewRegistry()
		var sut license.MetricsCollector
		sut.Entitlements = entitlements.New()
		sut.Entitlements.Modify(func(entitlements *codersdk.Entitlements) {
			entitlements.Errors = []string{
				"License has expired",
			}
		})

		registry.Register(&sut)

		metrics, err := registry.Gather()
		require.NoError(t, err)

		warnings, ok := findMetric(metrics, "coderd_license_warnings")
		require.True(t, ok)
		require.Zero(t, warnings)

		errors, ok := findMetric(metrics, "coderd_license_errors")
		require.True(t, ok)
		require.Equal(t, 1, errors)
	})

	t.Run("WithBothWarningsAndErrors", func(t *testing.T) {
		t.Parallel()

		registry := prometheus.NewRegistry()
		var sut license.MetricsCollector
		sut.Entitlements = entitlements.New()
		sut.Entitlements.Modify(func(entitlements *codersdk.Entitlements) {
			entitlements.Warnings = []string{
				"License expires in 7 days",
				"User limit is at 95% capacity",
				"Feature X is deprecated",
			}
			entitlements.Errors = []string{
				"Invalid license signature",
				"License UUID mismatch",
			}
		})

		registry.Register(&sut)

		metrics, err := registry.Gather()
		require.NoError(t, err)

		warnings, ok := findMetric(metrics, "coderd_license_warnings")
		require.True(t, ok)
		require.Equal(t, 3, warnings)

		errors, ok := findMetric(metrics, "coderd_license_errors")
		require.True(t, ok)
		require.Equal(t, 2, errors)
	})
}

// findMetric searches for a metric by name and returns its value.
func findMetric(metrics []*prometheus_client.MetricFamily, name string) (int, bool) {
	for _, metric := range metrics {
		if metric.GetName() == name {
			for _, m := range metric.Metric {
				return int(m.Gauge.GetValue()), true
			}
		}
	}
	return 0, false
}
