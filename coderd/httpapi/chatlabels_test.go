package httpapi_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpapi"
)

func TestValidateChatLabels(t *testing.T) {
	t.Parallel()

	t.Run("NilMap", func(t *testing.T) {
		t.Parallel()
		errs := httpapi.ValidateChatLabels(nil)
		require.Empty(t, errs)
	})

	t.Run("EmptyMap", func(t *testing.T) {
		t.Parallel()
		errs := httpapi.ValidateChatLabels(map[string]string{})
		require.Empty(t, errs)
	})

	t.Run("ValidLabels", func(t *testing.T) {
		t.Parallel()
		labels := map[string]string{
			"env":            "production",
			"github.repo":    "coder/coder",
			"automation/pr":  "12345",
			"team-backend":   "core",
			"version_number": "v1.2.3",
			"A1.b2/c3-d4_e5": "mixed",
		}
		errs := httpapi.ValidateChatLabels(labels)
		require.Empty(t, errs)
	})

	t.Run("TooManyLabels", func(t *testing.T) {
		t.Parallel()
		labels := make(map[string]string, 51)
		for i := range 51 {
			labels[strings.Repeat("k", i+1)] = "v"
		}
		errs := httpapi.ValidateChatLabels(labels)
		require.NotEmpty(t, errs)

		found := false
		for _, e := range errs {
			if strings.Contains(e.Detail, "too many labels") {
				found = true
				break
			}
		}
		assert.True(t, found, "expected a 'too many labels' error")
	})

	t.Run("KeyTooLong", func(t *testing.T) {
		t.Parallel()
		longKey := strings.Repeat("a", 65)
		labels := map[string]string{
			longKey: "value",
		}
		errs := httpapi.ValidateChatLabels(labels)
		require.NotEmpty(t, errs)

		found := false
		for _, e := range errs {
			if strings.Contains(e.Detail, "exceeds maximum length of 64 bytes") {
				found = true
				break
			}
		}
		assert.True(t, found, "expected a key-too-long error")
	})

	t.Run("ValueTooLong", func(t *testing.T) {
		t.Parallel()
		longValue := strings.Repeat("v", 257)
		labels := map[string]string{
			"key": longValue,
		}
		errs := httpapi.ValidateChatLabels(labels)
		require.NotEmpty(t, errs)

		found := false
		for _, e := range errs {
			if strings.Contains(e.Detail, "exceeds maximum length of 256 bytes") {
				found = true
				break
			}
		}
		assert.True(t, found, "expected a value-too-long error")
	})

	t.Run("InvalidKeyWithSpaces", func(t *testing.T) {
		t.Parallel()
		labels := map[string]string{
			"invalid key": "value",
		}
		errs := httpapi.ValidateChatLabels(labels)
		require.NotEmpty(t, errs)

		found := false
		for _, e := range errs {
			if strings.Contains(e.Detail, "contains invalid characters") {
				found = true
				break
			}
		}
		assert.True(t, found, "expected an invalid-characters error for spaces")
	})

	t.Run("InvalidKeyWithSpecialChars", func(t *testing.T) {
		t.Parallel()
		labels := map[string]string{
			"key@value": "value",
		}
		errs := httpapi.ValidateChatLabels(labels)
		require.NotEmpty(t, errs)

		found := false
		for _, e := range errs {
			if strings.Contains(e.Detail, "contains invalid characters") {
				found = true
				break
			}
		}
		assert.True(t, found, "expected an invalid-characters error for special chars")
	})

	t.Run("KeyStartsWithNonAlphanumeric", func(t *testing.T) {
		t.Parallel()
		labels := map[string]string{
			".dotfirst":   "value",
			"-dashfirst":  "value",
			"_underfirst": "value",
			"/slashfirst": "value",
		}
		errs := httpapi.ValidateChatLabels(labels)
		// Each of the four keys should produce an error.
		require.Len(t, errs, 4)
		for _, e := range errs {
			assert.Contains(t, e.Detail, "contains invalid characters")
		}
	})

	t.Run("EmptyKey", func(t *testing.T) {
		t.Parallel()
		labels := map[string]string{
			"": "value",
		}
		errs := httpapi.ValidateChatLabels(labels)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, "must not be empty")
	})

	t.Run("EmptyValue", func(t *testing.T) {
		t.Parallel()
		labels := map[string]string{
			"key": "",
		}
		errs := httpapi.ValidateChatLabels(labels)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, "must not be empty")
	})

	t.Run("AllFieldsAreLabels", func(t *testing.T) {
		t.Parallel()
		labels := map[string]string{
			"bad key": "",
		}
		errs := httpapi.ValidateChatLabels(labels)
		for _, e := range errs {
			assert.Equal(t, "labels", e.Field)
		}
	})

	t.Run("ExactlyAtLimits", func(t *testing.T) {
		t.Parallel()
		// Keys and values exactly at their limits should be valid.
		labels := map[string]string{
			strings.Repeat("a", 64): strings.Repeat("v", 256),
		}
		errs := httpapi.ValidateChatLabels(labels)
		require.Empty(t, errs)
	})
}
