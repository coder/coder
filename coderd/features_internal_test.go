package coderd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/codersdk"
)

func TestEntitlements(t *testing.T) {
	t.Parallel()
	t.Run("GET", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest("GET", "https://example.com/api/v2/entitlements", nil)
		rw := httptest.NewRecorder()
		entitlements(rw, r)
		resp := rw.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		dec := json.NewDecoder(resp.Body)
		var result codersdk.Entitlements
		err := dec.Decode(&result)
		require.NoError(t, err)
		assert.False(t, result.HasLicense)
		assert.Empty(t, result.Warnings)
		for _, f := range codersdk.FeatureNames {
			require.Contains(t, result.Features, f)
			fe := result.Features[f]
			assert.False(t, fe.Enabled)
			assert.Equal(t, codersdk.EntitlementNotEntitled, fe.Entitlement)
		}
	})
}
