package dynamicparameters

import (
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"
)

func TestRenderCache_BasicOperations(t *testing.T) {
	t.Parallel()

	cache := NewRenderCache()
	templateVersionID := uuid.New()
	ownerID := uuid.New()
	params := map[string]string{"region": "us-west-2"}

	// Cache should be empty initially
	_, ok := cache.get(templateVersionID, ownerID, params)
	require.False(t, ok, "cache should be empty initially")

	// Put an entry in the cache
	expectedOutput := &preview.Output{
		Parameters: []previewtypes.Parameter{
			{
				ParameterData: previewtypes.ParameterData{
					Name: "region",
					Type: previewtypes.ParameterTypeString,
				},
			},
		},
	}
	cache.put(templateVersionID, ownerID, params, expectedOutput)

	// Get should now return the cached value
	cachedOutput, ok := cache.get(templateVersionID, ownerID, params)
	require.True(t, ok, "cache should contain the entry")
	require.Same(t, expectedOutput, cachedOutput, "should return same pointer")
}

func TestRenderCache_DifferentKeysAreSeparate(t *testing.T) {
	t.Parallel()

	cache := NewRenderCache()
	templateVersion1 := uuid.New()
	templateVersion2 := uuid.New()
	owner1 := uuid.New()
	owner2 := uuid.New()
	params := map[string]string{"region": "us-west-2"}

	output1 := &preview.Output{}
	output2 := &preview.Output{}
	output3 := &preview.Output{}

	// Put different entries for different keys
	cache.put(templateVersion1, owner1, params, output1)
	cache.put(templateVersion2, owner1, params, output2)
	cache.put(templateVersion1, owner2, params, output3)

	// Verify each key returns its own entry
	cached1, ok1 := cache.get(templateVersion1, owner1, params)
	require.True(t, ok1)
	require.Same(t, output1, cached1)

	cached2, ok2 := cache.get(templateVersion2, owner1, params)
	require.True(t, ok2)
	require.Same(t, output2, cached2)

	cached3, ok3 := cache.get(templateVersion1, owner2, params)
	require.True(t, ok3)
	require.Same(t, output3, cached3)
}

func TestRenderCache_ParameterHashConsistency(t *testing.T) {
	t.Parallel()

	cache := NewRenderCache()
	templateVersionID := uuid.New()
	ownerID := uuid.New()

	// Parameters in different order should produce same cache key
	params1 := map[string]string{"a": "1", "b": "2", "c": "3"}
	params2 := map[string]string{"c": "3", "a": "1", "b": "2"}

	output := &preview.Output{}
	cache.put(templateVersionID, ownerID, params1, output)

	// Should hit cache even with different parameter order
	cached, ok := cache.get(templateVersionID, ownerID, params2)
	require.True(t, ok, "different parameter order should still hit cache")
	require.Same(t, output, cached)
}

func TestRenderCache_EmptyParameters(t *testing.T) {
	t.Parallel()

	cache := NewRenderCache()
	templateVersionID := uuid.New()
	ownerID := uuid.New()

	// Test with empty parameters
	emptyParams := map[string]string{}
	output := &preview.Output{}

	cache.put(templateVersionID, ownerID, emptyParams, output)

	cached, ok := cache.get(templateVersionID, ownerID, emptyParams)
	require.True(t, ok)
	require.Same(t, output, cached)
}

func TestRenderCache_PrebuildScenario(t *testing.T) {
	t.Parallel()

	// This test simulates the prebuild scenario where multiple prebuilds
	// are created from the same template version with the same preset parameters.
	cache := NewRenderCache()

	// In prebuilds, all instances use the same fixed ownerID
	prebuildOwnerID := uuid.MustParse("c42fdf75-3097-471c-8c33-fb52454d81c0") // database.PrebuildsSystemUserID
	templateVersionID := uuid.New()
	presetParams := map[string]string{
		"instance_type": "t3.micro",
		"region":        "us-west-2",
	}

	output := &preview.Output{}

	// First prebuild - cache miss
	_, ok := cache.get(templateVersionID, prebuildOwnerID, presetParams)
	require.False(t, ok, "first prebuild should miss cache")

	cache.put(templateVersionID, prebuildOwnerID, presetParams, output)

	// Second prebuild with same template version and preset - cache hit
	cached2, ok2 := cache.get(templateVersionID, prebuildOwnerID, presetParams)
	require.True(t, ok2, "second prebuild should hit cache")
	require.Same(t, output, cached2, "should return cached output")

	// Third prebuild - also cache hit
	cached3, ok3 := cache.get(templateVersionID, prebuildOwnerID, presetParams)
	require.True(t, ok3, "third prebuild should hit cache")
	require.Same(t, output, cached3, "should return cached output")

	// All three prebuilds shared the same cache entry
}

func TestRenderCache_Metrics(t *testing.T) {
	t.Parallel()

	// Create test metrics
	cacheHits := &testCounter{}
	cacheMisses := &testCounter{}
	cacheSize := &testGauge{}

	cache := NewRenderCacheWithMetrics(cacheHits, cacheMisses, cacheSize)
	templateVersionID := uuid.New()
	ownerID := uuid.New()
	params := map[string]string{"region": "us-west-2"}

	// Initially: 0 hits, 0 misses, 0 size
	require.Equal(t, float64(0), cacheHits.value, "initial hits should be 0")
	require.Equal(t, float64(0), cacheMisses.value, "initial misses should be 0")
	require.Equal(t, float64(0), cacheSize.value, "initial size should be 0")

	// First get - should be a miss
	_, ok := cache.get(templateVersionID, ownerID, params)
	require.False(t, ok)
	require.Equal(t, float64(0), cacheHits.value, "hits should still be 0")
	require.Equal(t, float64(1), cacheMisses.value, "misses should be 1")
	require.Equal(t, float64(0), cacheSize.value, "size should still be 0")

	// Put an entry
	output := &preview.Output{}
	cache.put(templateVersionID, ownerID, params, output)
	require.Equal(t, float64(1), cacheSize.value, "size should be 1 after put")

	// Second get - should be a hit
	_, ok = cache.get(templateVersionID, ownerID, params)
	require.True(t, ok)
	require.Equal(t, float64(1), cacheHits.value, "hits should be 1")
	require.Equal(t, float64(1), cacheMisses.value, "misses should still be 1")
	require.Equal(t, float64(1), cacheSize.value, "size should still be 1")

	// Third get - another hit
	_, ok = cache.get(templateVersionID, ownerID, params)
	require.True(t, ok)
	require.Equal(t, float64(2), cacheHits.value, "hits should be 2")
	require.Equal(t, float64(1), cacheMisses.value, "misses should still be 1")

	// Put another entry with different params
	params2 := map[string]string{"region": "us-east-1"}
	cache.put(templateVersionID, ownerID, params2, output)
	require.Equal(t, float64(2), cacheSize.value, "size should be 2 after second put")

	// Get with different params - should be a hit
	_, ok = cache.get(templateVersionID, ownerID, params2)
	require.True(t, ok)
	require.Equal(t, float64(3), cacheHits.value, "hits should be 3")
	require.Equal(t, float64(1), cacheMisses.value, "misses should still be 1")
}

// Test implementations of prometheus interfaces
type testCounter struct {
	value float64
}

func (c *testCounter) Inc() {
	c.value++
}

func (c *testCounter) Add(v float64) {
	c.value += v
}

func (c *testCounter) Desc() *prometheus.Desc {
	return nil
}

func (c *testCounter) Write(*dto.Metric) error {
	return nil
}

func (c *testCounter) Describe(chan<- *prometheus.Desc) {}

func (c *testCounter) Collect(chan<- prometheus.Metric) {}

type testGauge struct {
	value float64
}

func (g *testGauge) Set(v float64) {
	g.value = v
}

func (g *testGauge) Inc() {
	g.value++
}

func (g *testGauge) Dec() {
	g.value--
}

func (g *testGauge) Add(v float64) {
	g.value += v
}

func (g *testGauge) Sub(v float64) {
	g.value -= v
}

func (g *testGauge) SetToCurrentTime() {}

func (g *testGauge) Desc() *prometheus.Desc {
	return nil
}

func (g *testGauge) Write(*dto.Metric) error {
	return nil
}

func (g *testGauge) Describe(chan<- *prometheus.Desc) {}

func (g *testGauge) Collect(chan<- prometheus.Metric) {}
