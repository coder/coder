package dynamicparameters_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/preview"
)

func TestHashParameterValues(t *testing.T) {
	t.Parallel()

	t.Run("EmptyMap", func(t *testing.T) {
		t.Parallel()
		hash1 := dynamicparameters.HashParameterValues(map[string]string{})
		hash2 := dynamicparameters.HashParameterValues(map[string]string{})
		require.Equal(t, hash1, hash2, "empty maps should have the same hash")
	})

	t.Run("SameValuesDifferentOrder", func(t *testing.T) {
		t.Parallel()
		// Go maps iterate in random order, but the hash should be deterministic
		values := map[string]string{
			"param1": "value1",
			"param2": "value2",
			"param3": "value3",
		}
		hash1 := dynamicparameters.HashParameterValues(values)
		hash2 := dynamicparameters.HashParameterValues(values)
		require.Equal(t, hash1, hash2, "same values should produce the same hash")
	})

	t.Run("DifferentValues", func(t *testing.T) {
		t.Parallel()
		hash1 := dynamicparameters.HashParameterValues(map[string]string{"param1": "value1"})
		hash2 := dynamicparameters.HashParameterValues(map[string]string{"param1": "value2"})
		require.NotEqual(t, hash1, hash2, "different values should produce different hashes")
	})

	t.Run("DifferentKeys", func(t *testing.T) {
		t.Parallel()
		hash1 := dynamicparameters.HashParameterValues(map[string]string{"param1": "value"})
		hash2 := dynamicparameters.HashParameterValues(map[string]string{"param2": "value"})
		require.NotEqual(t, hash1, hash2, "different keys should produce different hashes")
	})

	t.Run("NoCollisionBetweenKeyAndValue", func(t *testing.T) {
		t.Parallel()
		// Ensure "key=a, value=b" doesn't hash the same as "key=ab, value="
		hash1 := dynamicparameters.HashParameterValues(map[string]string{"a": "b"})
		hash2 := dynamicparameters.HashParameterValues(map[string]string{"ab": ""})
		require.NotEqual(t, hash1, hash2, "key/value boundaries should not cause collisions")
	})
}

func TestPreviewCache(t *testing.T) {
	t.Parallel()

	t.Run("GetMiss", func(t *testing.T) {
		t.Parallel()
		cache := dynamicparameters.NewPreviewCache()
		key := dynamicparameters.PreviewCacheKey{
			TemplateVersionID: uuid.New(),
			OwnerID:           uuid.New(),
			ValuesHash:        dynamicparameters.HashParameterValues(map[string]string{}),
		}
		_, ok := cache.Get(key)
		require.False(t, ok, "cache should miss on first access")
	})

	t.Run("SetAndGet", func(t *testing.T) {
		t.Parallel()
		cache := dynamicparameters.NewPreviewCache()
		key := dynamicparameters.PreviewCacheKey{
			TemplateVersionID: uuid.New(),
			OwnerID:           uuid.New(),
			ValuesHash:        dynamicparameters.HashParameterValues(map[string]string{"param": "value"}),
		}
		output := &preview.Output{}
		cache.Set(key, output)

		cached, ok := cache.Get(key)
		require.True(t, ok, "cache should hit after set")
		require.Same(t, output, cached, "cached value should be the same pointer")
	})

	t.Run("DifferentKeysMiss", func(t *testing.T) {
		t.Parallel()
		cache := dynamicparameters.NewPreviewCache()
		key1 := dynamicparameters.PreviewCacheKey{
			TemplateVersionID: uuid.New(),
			OwnerID:           uuid.New(),
			ValuesHash:        dynamicparameters.HashParameterValues(map[string]string{"param": "value1"}),
		}
		key2 := dynamicparameters.PreviewCacheKey{
			TemplateVersionID: key1.TemplateVersionID,
			OwnerID:           key1.OwnerID,
			ValuesHash:        dynamicparameters.HashParameterValues(map[string]string{"param": "value2"}),
		}

		output := &preview.Output{}
		cache.Set(key1, output)

		_, ok := cache.Get(key2)
		require.False(t, ok, "different keys should not hit")
	})

	t.Run("DifferentTemplateVersionMiss", func(t *testing.T) {
		t.Parallel()
		cache := dynamicparameters.NewPreviewCache()
		valuesHash := dynamicparameters.HashParameterValues(map[string]string{"param": "value"})
		ownerID := uuid.New()

		key1 := dynamicparameters.PreviewCacheKey{
			TemplateVersionID: uuid.New(),
			OwnerID:           ownerID,
			ValuesHash:        valuesHash,
		}
		key2 := dynamicparameters.PreviewCacheKey{
			TemplateVersionID: uuid.New(), // Different template version
			OwnerID:           ownerID,
			ValuesHash:        valuesHash,
		}

		output := &preview.Output{}
		cache.Set(key1, output)

		_, ok := cache.Get(key2)
		require.False(t, ok, "different template versions should not hit")
	})

	t.Run("DifferentOwnerMiss", func(t *testing.T) {
		t.Parallel()
		cache := dynamicparameters.NewPreviewCache()
		valuesHash := dynamicparameters.HashParameterValues(map[string]string{"param": "value"})
		templateVersionID := uuid.New()

		key1 := dynamicparameters.PreviewCacheKey{
			TemplateVersionID: templateVersionID,
			OwnerID:           uuid.New(),
			ValuesHash:        valuesHash,
		}
		key2 := dynamicparameters.PreviewCacheKey{
			TemplateVersionID: templateVersionID,
			OwnerID:           uuid.New(), // Different owner
			ValuesHash:        valuesHash,
		}

		output := &preview.Output{}
		cache.Set(key1, output)

		_, ok := cache.Get(key2)
		require.False(t, ok, "different owners should not hit")
	})
}
