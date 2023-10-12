package license_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/aws/smithy-go/ptr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestCollectLicenseMetrics(t *testing.T) {
	t.Parallel()

	// Given
	registry := prometheus.NewRegistry()

	sut := license.NewMetricsCollector()

	const (
		actualUsers = 4
		userLimit   = 7
	)
	sut.Entitlements.Store(&codersdk.Entitlements{
		Features: map[codersdk.FeatureName]codersdk.Feature{
			codersdk.FeatureUserLimit: {
				Enabled: true,
				Actual:  ptr.Int64(actualUsers),
				Limit:   ptr.Int64(userLimit),
			},
		},
	})

	// When
	closeFunc, err := sut.Collect(ctx)
	require.NoError(t, err)
	t.Cleanup(closeFunc)

	// Then
	goldenFile, err := os.ReadFile("testdata/license-metrics.json")
	require.NoError(t, err)
	golden := map[string]int{}
	err = json.Unmarshal(goldenFile, &golden)
	require.NoError(t, err)

	collected := map[string]int{}

	assert.Eventually(t, func() bool {
		metrics, err := registry.Gather()
		assert.NoError(t, err)

		if len(metrics) < 1 {
			return false
		}

		for _, metric := range metrics {
			switch metric.GetName() {
			case "coderd_license_active_users", "coderd_license_user_limit":
				for _, m := range metric.Metric {
					collected[m.Label[0].GetName()+"="+m.Label[0].GetValue()+":"+metric.GetName()] = int(m.Gauge.GetValue())
				}
			default:
				require.FailNowf(t, "unexpected metric collected", "metric: %s", metric.GetName())
			}
		}
		return reflect.DeepEqual(golden, collected)
	}, testutil.WaitShort, testutil.IntervalFast)

	assert.EqualValues(t, golden, collected)
}
