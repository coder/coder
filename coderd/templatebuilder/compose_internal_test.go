package templatebuilder

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeModuleVariables(t *testing.T) {
	t.Parallel()

	manifest := ModuleManifest{
		Variables: []ModuleVariable{
			{Name: "agent_id", Type: "string", Computed: true},
			{Name: "api_key", Type: "string", Sensitive: true},
			{Name: "port", Type: "number", Default: json.RawMessage(`13337`)},
			{Name: "enabled", Type: "bool", Default: json.RawMessage(`false`)},
			{Name: "optional_no_default", Type: "string", Required: false},
			{Name: "required_no_default", Type: "string", Required: true},
		},
	}

	requiredVars := map[string]string{
		"required_no_default": "value",
	}

	t.Run("DefaultsApplied", func(t *testing.T) {
		t.Parallel()
		merged, err := mergeModuleVariables(manifest, requiredVars)
		require.NoError(t, err)
		require.Equal(t, "13337", merged["port"])
		require.Equal(t, "false", merged["enabled"])
	})

	t.Run("ComputedAndSensitiveSkipped", func(t *testing.T) {
		t.Parallel()
		merged, err := mergeModuleVariables(manifest, requiredVars)
		require.NoError(t, err)
		require.NotContains(t, merged, "agent_id")
		require.NotContains(t, merged, "api_key")
	})

	t.Run("NonRequiredWithoutDefaultGetsNull", func(t *testing.T) {
		t.Parallel()
		merged, err := mergeModuleVariables(manifest, requiredVars)
		require.NoError(t, err)
		require.Equal(t, "null", merged["optional_no_default"])
	})

	t.Run("RequiredWithoutDefaultIsRequired", func(t *testing.T) {
		t.Parallel()
		_, err := mergeModuleVariables(manifest, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), `variable "required_no_default"`)
		require.Contains(t, err.Error(), "is required")
	})

	t.Run("CallerOverridesDefault", func(t *testing.T) {
		t.Parallel()
		merged, err := mergeModuleVariables(manifest, map[string]string{
			"port":                "9999",
			"required_no_default": "value",
		})
		require.NoError(t, err)
		require.Equal(t, "9999", merged["port"])
	})

	t.Run("CallerProvidesRequired", func(t *testing.T) {
		t.Parallel()
		merged, err := mergeModuleVariables(manifest, map[string]string{
			"required_no_default": "value",
		})
		require.NoError(t, err)
		require.Equal(t, `"value"`, merged["required_no_default"])
	})

	t.Run("UnknownKeyRejected", func(t *testing.T) {
		t.Parallel()
		_, err := mergeModuleVariables(manifest, map[string]string{
			"nonexistent": `"val"`,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), `unknown variable "nonexistent"`)
	})

	t.Run("ComputedKeyRejected", func(t *testing.T) {
		t.Parallel()
		_, err := mergeModuleVariables(manifest, map[string]string{
			"agent_id": `"injected"`,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), `unknown variable "agent_id"`)
	})

	t.Run("SensitiveKeyRejected", func(t *testing.T) {
		t.Parallel()
		_, err := mergeModuleVariables(manifest, map[string]string{
			"api_key": `"secret"`,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), `unknown variable "api_key"`)
	})

	t.Run("InvalidNumberValueRejected", func(t *testing.T) {
		t.Parallel()
		_, err := mergeModuleVariables(manifest, map[string]string{
			"port": "abc",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), `variable "port"`)
		require.Contains(t, err.Error(), "invalid number value")
	})

	t.Run("InvalidBoolValueRejected", func(t *testing.T) {
		t.Parallel()
		_, err := mergeModuleVariables(manifest, map[string]string{
			"enabled": "yes",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), `variable "enabled"`)
		require.Contains(t, err.Error(), "invalid bool value")
	})

	t.Run("InvalidStringValueRejected", func(t *testing.T) {
		t.Parallel()
		_, err := mergeModuleVariables(manifest, map[string]string{
			"optional_no_default": "${var.foo}",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), `variable "optional_no_default"`)
		require.Contains(t, err.Error(), "interpolation")
	})

	t.Run("NullAcceptedForAnyType", func(t *testing.T) {
		t.Parallel()
		merged, err := mergeModuleVariables(manifest, map[string]string{
			"port":                "null",
			"enabled":             "null",
			"optional_no_default": "null",
			"required_no_default": "null",
		})
		require.NoError(t, err)
		require.Equal(t, "null", merged["port"])
		require.Equal(t, "null", merged["enabled"])
		require.Equal(t, "null", merged["optional_no_default"])
	})

	t.Run("EmptyCallerVarsUsesDefaults", func(t *testing.T) {
		t.Parallel()
		merged, err := mergeModuleVariables(manifest, map[string]string{
			"required_no_default": "value",
		})
		require.NoError(t, err)
		require.Equal(t, "13337", merged["port"])
	})
}
