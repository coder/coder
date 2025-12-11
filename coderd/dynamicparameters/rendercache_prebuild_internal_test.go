package dynamicparameters

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"
	"github.com/coder/terraform-provider-coder/v2/provider"
)

// TestRenderCache_PrebuildWithResolveParameters simulates the actual prebuild flow
// where ResolveParameters calls Render() twice - once with previous values and once
// with the final computed values.
func TestRenderCache_PrebuildWithResolveParameters(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	// Create test metrics
	cacheHits := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_prebuild_cache_hits_total",
	})
	cacheMisses := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_prebuild_cache_misses_total",
	})
	cacheSize := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test_prebuild_cache_entries",
	})

	cache := NewRenderCacheWithMetrics(cacheHits, cacheMisses, cacheSize)
	defer cache.Close()

	// Simulate prebuild scenario
	prebuildOwnerID := uuid.MustParse("c42fdf75-3097-471c-8c33-fb52454d81c0") // database.PrebuildsSystemUserID
	templateVersionID := uuid.New()

	// Preset parameters that all prebuilds share
	presetParams := []database.TemplateVersionPresetParameter{
		{Name: "instance_type", Value: "t3.micro"},
		{Name: "region", Value: "us-west-2"},
	}

	// Create a mock renderer that returns consistent parameter definitions
	mockRenderer := &mockRenderer{
		cache:             cache,
		templateVersionID: templateVersionID,
		output: &preview.Output{
			Parameters: []previewtypes.Parameter{
				{
					ParameterData: previewtypes.ParameterData{
						Name:         "instance_type",
						Type:         previewtypes.ParameterTypeString,
						FormType:     provider.ParameterFormTypeInput,
						Mutable:      true,
						DefaultValue: previewtypes.StringLiteral("t3.micro"),
						Required:     true,
					},
					Value:       previewtypes.StringLiteral("t3.micro"),
					Diagnostics: nil,
				},
				{
					ParameterData: previewtypes.ParameterData{
						Name:         "region",
						Type:         previewtypes.ParameterTypeString,
						FormType:     provider.ParameterFormTypeInput,
						Mutable:      true,
						DefaultValue: previewtypes.StringLiteral("us-west-2"),
						Required:     true,
					},
					Value:       previewtypes.StringLiteral("us-west-2"),
					Diagnostics: nil,
				},
			},
		},
	}

	// Initial metrics should be 0
	require.Equal(t, float64(0), promtestutil.ToFloat64(cacheHits), "initial hits should be 0")
	require.Equal(t, float64(0), promtestutil.ToFloat64(cacheMisses), "initial misses should be 0")
	require.Equal(t, float64(0), promtestutil.ToFloat64(cacheSize), "initial size should be 0")

	// === FIRST PREBUILD ===
	// First build: no previous values, preset values provided
	values1, err := ResolveParameters(ctx, prebuildOwnerID, mockRenderer, true,
		[]database.WorkspaceBuildParameter{}, // No previous values (first build)
		[]codersdk.WorkspaceBuildParameter{}, // No build-specific values
		presetParams,                         // Preset values from template
	)
	require.NoError(t, err)
	require.NotNil(t, values1)

	// After first prebuild:
	// - ResolveParameters calls Render() twice:
	//   1. With previousValuesMap (empty {})  → miss, creates cache entry
	//   2. With values.ValuesMap() ({preset}) → miss, creates cache entry
	// Expected: 0 hits, 2 misses, 2 cache entries
	t.Logf("After first prebuild: hits=%v, misses=%v, size=%v",
		promtestutil.ToFloat64(cacheHits),
		promtestutil.ToFloat64(cacheMisses),
		promtestutil.ToFloat64(cacheSize))

	require.Equal(t, float64(0), promtestutil.ToFloat64(cacheHits), "first prebuild should have 0 hits")
	require.Equal(t, float64(2), promtestutil.ToFloat64(cacheMisses), "first prebuild should have 2 misses")
	require.Equal(t, float64(2), promtestutil.ToFloat64(cacheSize), "should have 2 cache entries after first prebuild")

	// === SECOND PREBUILD ===
	// Second build: previous values now set to preset values
	previousValues := []database.WorkspaceBuildParameter{
		{Name: "instance_type", Value: "t3.micro"},
		{Name: "region", Value: "us-west-2"},
	}

	values2, err := ResolveParameters(ctx, prebuildOwnerID, mockRenderer, false,
		previousValues, // Previous values from first build
		[]codersdk.WorkspaceBuildParameter{},
		presetParams,
	)
	require.NoError(t, err)
	require.NotNil(t, values2)

	// After second prebuild:
	// - ResolveParameters calls Render() twice:
	//   1. With previousValuesMap ({preset}) → HIT (cache entry from first prebuild's 2nd render)
	//   2. With values.ValuesMap() ({preset})  → HIT (same cache entry)
	// Expected: 2 hits, 2 misses (still), 2 cache entries (still)
	t.Logf("After second prebuild: hits=%v, misses=%v, size=%v",
		promtestutil.ToFloat64(cacheHits),
		promtestutil.ToFloat64(cacheMisses),
		promtestutil.ToFloat64(cacheSize))

	require.Equal(t, float64(2), promtestutil.ToFloat64(cacheHits), "second prebuild should have 2 hits")
	require.Equal(t, float64(2), promtestutil.ToFloat64(cacheMisses), "misses should still be 2")
	require.Equal(t, float64(2), promtestutil.ToFloat64(cacheSize), "should still have 2 cache entries")

	// === THIRD PREBUILD ===
	values3, err := ResolveParameters(ctx, prebuildOwnerID, mockRenderer, false,
		previousValues,
		[]codersdk.WorkspaceBuildParameter{},
		presetParams,
	)
	require.NoError(t, err)
	require.NotNil(t, values3)

	// After third prebuild:
	// - ResolveParameters calls Render() twice:
	//   1. With previousValuesMap ({preset}) → HIT
	//   2. With values.ValuesMap() ({preset})  → HIT
	// Expected: 4 hits, 2 misses (still), 2 cache entries (still)
	t.Logf("After third prebuild: hits=%v, misses=%v, size=%v",
		promtestutil.ToFloat64(cacheHits),
		promtestutil.ToFloat64(cacheMisses),
		promtestutil.ToFloat64(cacheSize))

	require.Equal(t, float64(4), promtestutil.ToFloat64(cacheHits), "third prebuild should have 4 total hits")
	require.Equal(t, float64(2), promtestutil.ToFloat64(cacheMisses), "misses should still be 2")
	require.Equal(t, float64(2), promtestutil.ToFloat64(cacheSize), "should still have 2 cache entries")

	// Summary: With 3 prebuilds, we should have:
	// - 4 cache hits (2 from 2nd prebuild, 2 from 3rd prebuild)
	// - 2 cache misses (2 from 1st prebuild)
	// - 2 cache entries (one for empty params, one for preset params)
}

// mockRenderer is a simple renderer that uses the cache for testing
type mockRenderer struct {
	cache             RenderCache
	templateVersionID uuid.UUID
	output            *preview.Output
}

func (m *mockRenderer) Render(ctx context.Context, ownerID uuid.UUID, values map[string]string) (*preview.Output, hcl.Diagnostics) {
	// This simulates what dynamicRenderer does - check cache first
	if cached, ok := m.cache.get(m.templateVersionID, ownerID, values); ok {
		return cached, nil
	}

	// Not in cache, "render" (just return our mock output) and cache it
	m.cache.put(m.templateVersionID, ownerID, values, m.output)
	return m.output, nil
}

func (m *mockRenderer) Close() {}
