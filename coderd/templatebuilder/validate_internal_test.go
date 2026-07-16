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
