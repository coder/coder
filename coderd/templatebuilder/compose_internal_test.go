package templatebuilder

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/hcl/v2"
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

func TestValidateRenderedHCL(t *testing.T) {
	t.Parallel()

	t.Run("ValidHCL", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, validateRenderedHCL([]byte(`resource "coder_agent" "main" {}`)))
	})

	t.Run("InvalidHCL", func(t *testing.T) {
		t.Parallel()
		// Compose relies on this backstop to reject invalid rendered HCL rather than
		// shipping a broken template.
		err := validateRenderedHCL([]byte("resource {"))
		require.Error(t, err)
		require.ErrorContains(t, err, "not valid HCL")
		// It is identifiable as a server-side fault and exposes every diagnostic.
		require.ErrorIs(t, err, ErrRenderedHCLInvalid)
		var diag *hcl.Diagnostic
		require.ErrorAs(t, err, &diag)
	})
}

func TestComposeRendersValidHCL(t *testing.T) {
	t.Parallel()
	// Exercise the Compose call sites that validate rendered HCL: both the base
	// main.tf and the rendered modules.tf must parse.
	res, err := Compose(ComposeRequest{
		BaseTemplateID: "docker",
		Modules:        []ComposeModule{{ID: "code-server"}},
	})
	require.NoError(t, err)
	require.NoError(t, validateRenderedHCL(res.MainTF))
	require.NotEmpty(t, res.ModulesTF)
	require.NoError(t, validateRenderedHCL(res.ModulesTF))
}
