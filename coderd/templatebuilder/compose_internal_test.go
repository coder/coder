package templatebuilder

import (
	"encoding/json"
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

	t.Run("DefaultsApplied", func(t *testing.T) {
		t.Parallel()
		merged := mergeModuleVariables(manifest, nil)
		require.Equal(t, "13337", merged["port"])
		require.Equal(t, "false", merged["enabled"])
	})

	t.Run("ComputedAndSensitiveSkipped", func(t *testing.T) {
		t.Parallel()
		merged := mergeModuleVariables(manifest, nil)
		require.NotContains(t, merged, "agent_id")
		require.NotContains(t, merged, "api_key")
	})

	t.Run("NonRequiredWithoutDefaultGetsNull", func(t *testing.T) {
		t.Parallel()
		merged := mergeModuleVariables(manifest, nil)
		require.Equal(t, "null", merged["optional_no_default"])
	})

	t.Run("RequiredWithoutDefaultOmitted", func(t *testing.T) {
		t.Parallel()
		merged := mergeModuleVariables(manifest, nil)
		require.NotContains(t, merged, "required_no_default")
	})

	t.Run("CallerOverridesDefault", func(t *testing.T) {
		t.Parallel()
		merged := mergeModuleVariables(manifest, map[string]string{
			"port": "9999",
		})
		require.Equal(t, "9999", merged["port"])
	})

	t.Run("CallerProvidesRequired", func(t *testing.T) {
		t.Parallel()
		merged := mergeModuleVariables(manifest, map[string]string{
			"required_no_default": `"value"`,
		})
		require.Equal(t, `"value"`, merged["required_no_default"])
	})
}
