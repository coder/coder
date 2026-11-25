package chatprompt

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNeedsNulEncodingInJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  json.RawMessage
		want bool
	}{
		{name: "clean", raw: json.RawMessage(`{"value":"clean"}`)},
		{name: "nul", raw: json.RawMessage(`{"value":"\u0000"}`), want: true},
		{name: "escaped nul text", raw: json.RawMessage(`{"value":"\\u0000"}`)},
		{name: "escaped backslash then nul", raw: json.RawMessage(`{"value":"\\\u0000"}`), want: true},
		{name: "ordinary unicode escape", raw: json.RawMessage(`{"value":"\u1234"}`)},
		{name: "truncated unicode escape", raw: json.RawMessage(`{"value":"\u12"}`), want: true},
		{name: "unicode escape without digits", raw: json.RawMessage(`{"value":"\u`), want: true},
		{name: "invalid unicode escape", raw: json.RawMessage(`{"value":"\uZZZZ"}`), want: true},
		{name: "backslash outside string", raw: json.RawMessage(`\`)},
		{name: "trailing backslash in string", raw: json.RawMessage(`{"value":"\`)},

		{name: "two escaped backslashes then nul text", raw: json.RawMessage(`{"value":"\\\\u0000"}`)},
		{name: "escaped sentinel", raw: json.RawMessage(`{"value":"\uE000"}`), want: true},
		{name: "literal sentinel", raw: json.RawMessage(`{"value":""}`), want: true},
		{name: "valid surrogate pair", raw: json.RawMessage(`{"value":"\uD83D\uDE00"}`)},
		{name: "mixed case surrogate pair", raw: json.RawMessage(`{"value":"\ud83D\uDe00"}`)},
		{name: "lone high surrogate", raw: json.RawMessage(`{"value":"\uD800"}`), want: true},
		{name: "lone low surrogate", raw: json.RawMessage(`{"value":"\udc00"}`), want: true},
		{name: "reversed surrogate pair", raw: json.RawMessage(`{"value":"\uDC00\uD800"}`), want: true},
		{name: "surrogate literal text", raw: json.RawMessage(`{"value":"\\uD800"}`)},
		{name: "object key", raw: json.RawMessage(`{"key\u0000":"value"}`), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := needsNulEncodingInJSON(tt.raw)
			require.Equalf(t, tt.want, got, "needsNulEncodingInJSON(%s)", tt.raw)
		})
	}
}

func TestEncodeNulInJSONPreservesCleanInput(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"number":1.2300,"pair":"\uD83D\uDE00","literal":"\\u0000"}`)
	encoded := encodeNulInJSON(raw)
	require.Equal(t, raw, encoded, "clean JSON changed")
	require.Same(t, &raw[0], &encoded[0], "clean JSON did not preserve its backing array")
}

func TestEncodeNulInJSONNormalizesSurrogates(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"high":"\uD800","low":"\uDC00","pair":"\uD83D\uDE00","literal":"\\uD800"}`)
	encoded := encodeNulInJSON(raw)
	require.Truef(t, json.Valid(encoded), "encoded invalid JSON: %s", encoded)
	var got map[string]string
	require.NoError(t, json.Unmarshal(encoded, &got))
	want := map[string]string{
		"high":    "�",
		"low":     "�",
		"pair":    "😀",
		"literal": `\uD800`,
	}
	require.Equal(t, want, got, "normalized JSON mismatch")
}

func TestEncodeNulInJSONPreservesInvalidInput(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"value":"\uD800"} trailing`)
	encoded := encodeNulInJSON(raw)
	require.Equal(t, raw, encoded, "invalid JSON changed")
}
