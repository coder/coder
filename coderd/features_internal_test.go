package coderd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/codersdk"
)

func TestEntitlements(t *testing.T) {
	t.Parallel()
	t.Run("GET", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest("GET", "https://example.com/api/v2/entitlements", nil)
		rw := httptest.NewRecorder()
		(&featuresService{}).EntitlementsAPI(rw, r)
		resp := rw.Result()
		defer resp.Body.Close()
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

func TestFeaturesServiceGet(t *testing.T) {
	t.Parallel()
	t.Run("Auditor", func(t *testing.T) {
		t.Parallel()
		uut := featuresService{}
		target := struct {
			Auditor audit.Auditor
		}{}
		err := uut.Get(&target)
		require.NoError(t, err)
		assert.NotNil(t, target.Auditor)
	})

	t.Run("NotPointer", func(t *testing.T) {
		t.Parallel()
		uut := featuresService{}
		target := struct {
			Auditor audit.Auditor
		}{}
		err := uut.Get(target)
		require.Error(t, err)
		assert.Nil(t, target.Auditor)
	})

	t.Run("UnknownInterface", func(t *testing.T) {
		t.Parallel()
		uut := featuresService{}
		target := struct {
			test testInterface
		}{}
		err := uut.Get(&target)
		require.Error(t, err)
		assert.Nil(t, target.test)
	})

	t.Run("PointerToNonStruct", func(t *testing.T) {
		t.Parallel()
		uut := featuresService{}
		var target audit.Auditor
		err := uut.Get(&target)
		require.Error(t, err)
		assert.Nil(t, target)
	})

	t.Run("StructWithNonInterfaces", func(t *testing.T) {
		t.Parallel()
		uut := featuresService{}
		target := struct {
			N       int64
			Auditor audit.Auditor
		}{}
		err := uut.Get(&target)
		require.Error(t, err)
		assert.Nil(t, target.Auditor)
	})
}

type testInterface interface {
	Test() error
}
