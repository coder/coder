package coderd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCoerceEmailVerified(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    interface{}
		wantBool bool
		wantOK   bool
	}{
		// Native booleans
		{name: "BoolTrue", input: true, wantBool: true, wantOK: true},
		{name: "BoolFalse", input: false, wantBool: false, wantOK: true},

		// Strings
		{name: "StringTrue", input: "true", wantBool: true, wantOK: true},
		{name: "StringFalse", input: "false", wantBool: false, wantOK: true},
		{name: "StringOne", input: "1", wantBool: true, wantOK: true},
		{name: "StringZero", input: "0", wantBool: false, wantOK: true},
		{name: "StringTRUE", input: "TRUE", wantBool: true, wantOK: true},
		{name: "StringFALSE", input: "FALSE", wantBool: false, wantOK: true},
		{name: "StringT", input: "t", wantBool: true, wantOK: true},
		{name: "StringF", input: "f", wantBool: false, wantOK: true},
		{name: "StringInvalid", input: "invalid", wantBool: false, wantOK: false},
		{name: "StringEmpty", input: "", wantBool: false, wantOK: false},

		// json.Number (when decoder uses UseNumber)
		{name: "JSONNumberOne", input: json.Number("1"), wantBool: true, wantOK: true},
		{name: "JSONNumberZero", input: json.Number("0"), wantBool: false, wantOK: true},
		{name: "JSONNumberInvalid", input: json.Number("abc"), wantBool: false, wantOK: false},

		// float64 (default JSON numeric type)
		{name: "Float64One", input: float64(1), wantBool: true, wantOK: true},
		{name: "Float64Zero", input: float64(0), wantBool: false, wantOK: true},

		// Integer types
		{name: "IntOne", input: int(1), wantBool: true, wantOK: true},
		{name: "IntZero", input: int(0), wantBool: false, wantOK: true},
		{name: "Int64One", input: int64(1), wantBool: true, wantOK: true},
		{name: "Int64Zero", input: int64(0), wantBool: false, wantOK: true},

		// Nil and unsupported types
		{name: "Nil", input: nil, wantBool: false, wantOK: false},
		{name: "Slice", input: []string{}, wantBool: false, wantOK: false},
		{name: "Map", input: map[string]string{}, wantBool: false, wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotBool, gotOK := coerceEmailVerified(tc.input)
			assert.Equal(t, tc.wantBool, gotBool, "bool value mismatch")
			assert.Equal(t, tc.wantOK, gotOK, "ok value mismatch")
		})
	}
}
