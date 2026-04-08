package chatloop

import (
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testProviderData implements fantasy.ProviderOptionsData so we can
// construct arbitrary ProviderMetadata for extractContextLimit tests.
type testProviderData struct {
	data map[string]any
}

func (*testProviderData) Options() {}

func (d *testProviderData) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.data)
}

// Required by the ProviderOptionsData interface; unused in tests.
func (d *testProviderData) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &d.data)
}

func TestNormalizeMetadataKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "lowercase", key: "camelCase", want: "camelcase"},
		{name: "hyphens stripped", key: "kebab-case", want: "kebabcase"},
		{name: "underscores stripped", key: "snake_case", want: "snakecase"},
		{name: "uppercase", key: "UPPER", want: "upper"},
		{name: "spaces stripped", key: "with spaces", want: "withspaces"},
		{name: "empty", key: "", want: ""},
		{name: "digits preserved", key: "123", want: "123"},
		{name: "mixed separators", key: "Max_Context-Tokens", want: "maxcontexttokens"},
		{name: "dots stripped", key: "context.limit", want: "contextlimit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeMetadataKey(tt.key)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestMetadataKeyWords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key  string
		want []string
	}{
		{"max_context_tokens", []string{"max", "context", "tokens"}},
		{"maxContextTokens", []string{"max", "context", "tokens"}},
		{"MAX_CONTEXT", []string{"max", "context"}},
		{"ContextWindow", []string{"context", "window"}},
		{"context2limit", []string{"context", "limit"}},
		{"", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()
			got := metadataKeyWords(tt.key)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestIsContextLimitKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
		want bool
	}{ // Exact matches after normalization.
		{name: "context_limit", key: "context_limit", want: true},
		{name: "context_window", key: "context_window", want: true},
		{name: "context_length", key: "context_length", want: true},
		{name: "max_context", key: "max_context", want: true},
		{name: "max_context_tokens", key: "max_context_tokens", want: true},
		{name: "max_input_tokens", key: "max_input_tokens", want: true},
		{name: "max_input_token", key: "max_input_token", want: true},
		{name: "input_token_limit", key: "input_token_limit", want: true},

		// Case and separator variations.
		{name: "Context-Window mixed case", key: "Context-Window", want: true},
		{name: "MAX_CONTEXT_TOKENS screaming", key: "MAX_CONTEXT_TOKENS", want: true},
		{name: "contextLimit camelCase", key: "contextLimit", want: true},
		{name: "modelContextLimit camelCase", key: "modelContextLimit", want: true},

		// Fallback heuristic: tokenized "context" + limit/window/length.
		{name: "model_context_limit", key: "model_context_limit", want: true},
		{name: "context_window_size", key: "context_window_size", want: true},
		{name: "context_length_max", key: "context_length_max", want: true},

		// Exact matches remain valid after separator stripping.
		{name: "max_context_", key: "max_context_", want: true},
		{name: "max_context_limit", key: "max_context_limit", want: true},

		// Non-matching keys should not be treated as context limits.
		{name: "max_context_version false positive", key: "max_context_version", want: false},
		{name: "context_tokens_used false positive", key: "context_tokens_used", want: false},
		{name: "context_length_used false positive", key: "context_length_used", want: false},
		{name: "context_window_used false positive", key: "context_window_used", want: false},
		{name: "context_id no limit keyword", key: "context_id", want: false},
		{name: "empty string", key: "", want: false},
		{name: "unrelated key", key: "model_name", want: false},
		{name: "limit without context", key: "rate_limit", want: false},
		{name: "max without context", key: "max_tokens", want: false},
		{name: "context alone", key: "context", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isContextLimitKey(tt.key)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNumericContextLimitValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		value  any
		want   int64
		wantOK bool
	}{
		// float64: the default numeric type from json.Unmarshal.
		{name: "float64 integer", value: float64(128000), want: 128000, wantOK: true},
		{name: "float64 fractional rejected", value: float64(128000.5), want: 0, wantOK: false},
		{name: "float64 zero rejected", value: float64(0), want: 0, wantOK: false},
		{name: "float64 negative rejected", value: float64(-1), want: 0, wantOK: false},

		// int64
		{name: "int64 positive", value: int64(200000), want: 200000, wantOK: true},
		{name: "int64 zero rejected", value: int64(0), want: 0, wantOK: false},
		{name: "int64 negative rejected", value: int64(-1), want: 0, wantOK: false},

		// int32
		{name: "int32 positive", value: int32(50000), want: 50000, wantOK: true},
		{name: "int32 zero rejected", value: int32(0), want: 0, wantOK: false},

		// int
		{name: "int positive", value: int(50000), want: 50000, wantOK: true},
		{name: "int zero rejected", value: int(0), want: 0, wantOK: false},

		// string
		{name: "string numeric", value: "128000", want: 128000, wantOK: true},
		{name: "string trimmed", value: " 128000 ", want: 128000, wantOK: true},
		{name: "string non-numeric rejected", value: "not a number", want: 0, wantOK: false},
		{name: "string empty rejected", value: "", want: 0, wantOK: false},
		{name: "string zero rejected", value: "0", want: 0, wantOK: false},
		{name: "string negative rejected", value: "-1", want: 0, wantOK: false},

		// json.Number
		{name: "json.Number valid", value: json.Number("200000"), want: 200000, wantOK: true},
		{name: "json.Number invalid rejected", value: json.Number("invalid"), want: 0, wantOK: false},
		{name: "json.Number zero rejected", value: json.Number("0"), want: 0, wantOK: false},

		// Unhandled types.
		{name: "bool rejected", value: true, want: 0, wantOK: false},
		{name: "nil rejected", value: nil, want: 0, wantOK: false},
		{name: "slice rejected", value: []int{1}, want: 0, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := numericContextLimitValue(tt.value)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestPositiveInt64(t *testing.T) {
	t.Parallel()

	got, ok := positiveInt64(42)
	require.True(t, ok)
	require.Equal(t, int64(42), got)

	got, ok = positiveInt64(0)
	require.False(t, ok)
	require.Equal(t, int64(0), got)

	got, ok = positiveInt64(-1)
	require.False(t, ok)
	require.Equal(t, int64(0), got)
}

func TestCollectContextLimitValues(t *testing.T) {
	t.Parallel()

	t.Run("FlatMap", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"context_limit": float64(200000),
			"other_key":     float64(999),
		}
		var collected []int64
		collectContextLimitValues(input, func(v int64) {
			collected = append(collected, v)
		})
		require.Equal(t, []int64{200000}, collected)
	})

	t.Run("NestedMaps", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"provider": map[string]any{
				"info": map[string]any{
					"context_window": float64(100000),
				},
			},
		}
		var collected []int64
		collectContextLimitValues(input, func(v int64) {
			collected = append(collected, v)
		})
		require.Equal(t, []int64{100000}, collected)
	})

	t.Run("ArrayTraversal", func(t *testing.T) {
		t.Parallel()
		input := []any{
			map[string]any{"context_limit": float64(50000)},
			map[string]any{"context_limit": float64(80000)},
		}
		var collected []int64
		collectContextLimitValues(input, func(v int64) {
			collected = append(collected, v)
		})
		require.Len(t, collected, 2)
		require.Contains(t, collected, int64(50000))
		require.Contains(t, collected, int64(80000))
	})

	t.Run("MixedNesting", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"models": []any{
				map[string]any{
					"context_limit": float64(128000),
				},
			},
		}
		var collected []int64
		collectContextLimitValues(input, func(v int64) {
			collected = append(collected, v)
		})
		require.Equal(t, []int64{128000}, collected)
	})

	t.Run("NonMatchingKey", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"model_name": "gpt-4",
			"tokens":     float64(1000),
		}
		var collected []int64
		collectContextLimitValues(input, func(v int64) {
			collected = append(collected, v)
		})
		require.Empty(t, collected)
	})

	t.Run("ScalarIgnored", func(t *testing.T) {
		t.Parallel()
		var collected []int64
		collectContextLimitValues("just a string", func(v int64) {
			collected = append(collected, v)
		})
		require.Empty(t, collected)
	})
}

func TestFindContextLimitValue(t *testing.T) {
	t.Parallel()

	t.Run("SingleCandidate", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"context_limit": float64(200000),
		}
		limit, ok := findContextLimitValue(input)
		require.True(t, ok)
		require.Equal(t, int64(200000), limit)
	})

	t.Run("MultipleCandidatesTakesMax", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"a": map[string]any{"context_limit": float64(50000)},
			"b": map[string]any{"context_limit": float64(200000)},
		}
		limit, ok := findContextLimitValue(input)
		require.True(t, ok)
		require.Equal(t, int64(200000), limit)
	})

	t.Run("NoCandidates", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"model": "gpt-4",
		}
		_, ok := findContextLimitValue(input)
		require.False(t, ok)
	})

	t.Run("NilInput", func(t *testing.T) {
		t.Parallel()
		_, ok := findContextLimitValue(nil)
		require.False(t, ok)
	})
}

func TestExtractContextLimit(t *testing.T) {
	t.Parallel()

	t.Run("AnthropicStyle", func(t *testing.T) {
		t.Parallel()
		metadata := fantasy.ProviderMetadata{
			"anthropic": &testProviderData{
				data: map[string]any{
					"cache_read_input_tokens": float64(100),
					"context_limit":           float64(200000),
				},
			},
		}
		result := extractContextLimit(metadata)
		require.True(t, result.Valid)
		require.Equal(t, int64(200000), result.Int64)
	})

	t.Run("OpenAIStyle", func(t *testing.T) {
		t.Parallel()
		metadata := fantasy.ProviderMetadata{
			"openai": &testProviderData{
				data: map[string]any{
					"max_context_tokens": float64(128000),
				},
			},
		}
		result := extractContextLimit(metadata)
		require.True(t, result.Valid)
		require.Equal(t, int64(128000), result.Int64)
	})

	t.Run("NestedDeeply", func(t *testing.T) {
		t.Parallel()
		metadata := fantasy.ProviderMetadata{
			"provider": &testProviderData{
				data: map[string]any{
					"info": map[string]any{
						"context_window": float64(100000),
					},
				},
			},
		}
		result := extractContextLimit(metadata)
		require.True(t, result.Valid)
		require.Equal(t, int64(100000), result.Int64)
	})

	t.Run("MultipleCandidatesTakesMax", func(t *testing.T) {
		t.Parallel()
		metadata := fantasy.ProviderMetadata{
			"a": &testProviderData{
				data: map[string]any{
					"context_limit": float64(50000),
				},
			},
			"b": &testProviderData{
				data: map[string]any{
					"context_limit": float64(200000),
				},
			},
		}
		result := extractContextLimit(metadata)
		require.True(t, result.Valid)
		require.Equal(t, int64(200000), result.Int64)
	})

	t.Run("NoMatchingKeys", func(t *testing.T) {
		t.Parallel()
		metadata := fantasy.ProviderMetadata{
			"openai": &testProviderData{
				data: map[string]any{
					"model":  "gpt-4",
					"tokens": float64(1000),
				},
			},
		}
		result := extractContextLimit(metadata)
		assert.False(t, result.Valid)
	})

	t.Run("ContextUsageCountersIgnored", func(t *testing.T) {
		t.Parallel()
		metadata := fantasy.ProviderMetadata{
			"openai": &testProviderData{
				data: map[string]any{
					"context_tokens_used": float64(64000),
				},
			},
		}
		result := extractContextLimit(metadata)
		assert.False(t, result.Valid)
	})

	t.Run("NilMetadata", func(t *testing.T) {
		t.Parallel()
		result := extractContextLimit(nil)
		assert.False(t, result.Valid)
	})

	t.Run("EmptyMetadata", func(t *testing.T) {
		t.Parallel()
		result := extractContextLimit(fantasy.ProviderMetadata{})
		assert.False(t, result.Valid)
	})
}
