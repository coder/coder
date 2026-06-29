package templatebuilder

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsSimpleJSONValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  json.RawMessage
		want bool
	}{
		{"String", json.RawMessage(`"hello"`), true},
		{"EmptyString", json.RawMessage(`""`), true},
		{"True", json.RawMessage(`true`), true},
		{"False", json.RawMessage(`false`), true},
		{"Null", json.RawMessage(`null`), true},
		{"PositiveInt", json.RawMessage(`42`), true},
		{"NegativeInt", json.RawMessage(`-1`), true},
		{"Float", json.RawMessage(`3.14`), true},
		{"Array", json.RawMessage(`[1,2]`), false},
		{"Object", json.RawMessage(`{"a":1}`), false},
		{"Empty", json.RawMessage(``), false},
		{"Nil", nil, false},
		{"MalformedString", json.RawMessage(`"unclosed`), false},
		{"MalformedBool", json.RawMessage(`truesomething`), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isSimpleJSONValue(tc.raw)
			require.Equal(t, tc.want, got)
		})
	}
}

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

func TestValidateStringValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr string
	}{
		{"ValidEmpty", "", ""},
		{"ValidSimple", "hello", ""},
		{"ValidPath", "/home/coder", ""},
		{"ValidURL", "https://github.com/coder/coder", ""},
		{"ValidWithQuotes", `say "hi"`, ""},
		{"ValidWithNewlines", "line\nbreak", ""},
		{"ValidWithBackslash", `path\to\file`, ""},

		{"RejectedHCLInterpolation", "${var.foo}", "interpolation"},
		{"RejectedHCLDirective", "%{if true}yes%{endif}", "interpolation"},
		{"RejectedOverlong", strings.Repeat("a", maxStringValueLen+1), "maximum length"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateStringValue(tc.value)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestValidateNumberValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"Zero", "0", false},
		{"Positive", "42", false},
		{"Negative", "-1", false},
		{"Decimal", "3.14", false},
		{"NegativeDecimal", "-0.5", false},

		{"Scientific", "1e10", true},
		{"Hex", "0x1F", true},
		{"Underscore", "1_000", true},
		{"Expression", "1 + 1", true},
		{"Empty", "", true},
		{"Letters", "abc", true},
		{"TrailingDot", "1.", true},
		{"LeadingDot", ".5", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateNumberValue(tc.value)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateBoolValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"True", "true", false},
		{"False", "false", false},

		{"UpperTrue", "True", true},
		{"UpperFALSE", "FALSE", true},
		{"QuotedTrue", `"true"`, true},
		{"One", "1", true},
		{"Yes", "yes", true},
		{"Empty", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateBoolValue(tc.value)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateVariableValue(t *testing.T) {
	t.Parallel()

	t.Run("NullAcceptedForAllTypes", func(t *testing.T) {
		t.Parallel()
		for _, typ := range []string{"string", "number", "bool"} {
			t.Run(typ, func(t *testing.T) {
				t.Parallel()
				v := ModuleVariable{Name: "test", Type: typ}
				require.NoError(t, validateVariableValue(v, "null"))
			})
		}
	})

	t.Run("UnsupportedTypeRejected", func(t *testing.T) {
		t.Parallel()
		v := ModuleVariable{Name: "test", Type: "list"}
		err := validateVariableValue(v, "val")
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported variable type")
	})
}
