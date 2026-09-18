package db2sdk

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func TestAggregateTokenMetadata(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		rows []pqtype.NullRawMessage
		want map[string]any
	}{
		{
			name: "empty_input",
			want: map[string]any{},
		},
		{
			name: "sums_across_rows",
			rows: []pqtype.NullRawMessage{
				{RawMessage: json.RawMessage(`{"cache_read_tokens":100,"reasoning_tokens":50}`), Valid: true},
				{RawMessage: json.RawMessage(`{"cache_read_tokens":200,"reasoning_tokens":75}`), Valid: true},
			},
			want: map[string]any{
				"cache_read_tokens": int64(300),
				"reasoning_tokens":  int64(125),
			},
		},
		{
			name: "skips_null_and_invalid_metadata",
			rows: []pqtype.NullRawMessage{
				{Valid: false},
				{RawMessage: nil, Valid: true},
				{RawMessage: json.RawMessage(`{"tokens":42}`), Valid: true},
			},
			want: map[string]any{"tokens": int64(42)},
		},
		{
			// Float values fail json.Number.Int64(), so they are silently
			// dropped.
			name: "skips_non_integer_values",
			rows: []pqtype.NullRawMessage{
				{RawMessage: json.RawMessage(`{"good":10,"fractional":1.5}`), Valid: true},
			},
			want: map[string]any{"good": int64(10)},
		},
		{
			// The malformed row is skipped, the valid one is counted.
			name: "skips_malformed_json",
			rows: []pqtype.NullRawMessage{
				{RawMessage: json.RawMessage(`not json`), Valid: true},
				{RawMessage: json.RawMessage(`{"tokens":5}`), Valid: true},
			},
			want: map[string]any{"tokens": int64(5)},
		},
		{
			// Arrays are skipped.
			name: "flattens_nested_objects",
			rows: []pqtype.NullRawMessage{
				{RawMessage: json.RawMessage(`{
					"cache_read_tokens": 100,
					"cache": {"creation_tokens": 40, "read_tokens": 60},
					"reasoning_tokens": 50,
					"tags": ["a", "b"]
				}`), Valid: true},
				{RawMessage: json.RawMessage(`{
					"cache_read_tokens": 200,
					"cache": {"creation_tokens": 10}
				}`), Valid: true},
			},
			want: map[string]any{
				"cache_read_tokens":     int64(300),
				"reasoning_tokens":      int64(50),
				"cache.creation_tokens": int64(50),
				"cache.read_tokens":     int64(60),
			},
		},
		{
			name: "flattens_deeply_nested_objects",
			rows: []pqtype.NullRawMessage{
				{RawMessage: json.RawMessage(`{
					"provider": {
						"anthropic": {"cache_creation_tokens": 100, "cache_read_tokens": 200},
						"openai": {"reasoning_tokens": 50}
					},
					"total": 500
				}`), Valid: true},
			},
			want: map[string]any{
				"provider.anthropic.cache_creation_tokens": int64(100),
				"provider.anthropic.cache_read_tokens":     int64(200),
				"provider.openai.reasoning_tokens":         int64(50),
				"total":                                    int64(500),
			},
		},
		{
			// Real-world provider metadata shapes from
			// https://github.com/coder/aibridge/issues/150: Anthropic keeps
			// cache fields top-level and two rows sum, OpenAI nests them
			// inside input_tokens_details and they flatten with dot notation.
			name: "aggregates_real_provider_metadata",
			rows: []pqtype.NullRawMessage{
				{RawMessage: json.RawMessage(`{
					"cache_creation_input_tokens": 0,
					"cache_read_input_tokens": 23490
				}`), Valid: true},
				{RawMessage: json.RawMessage(`{
					"input_tokens_details": {"cached_tokens": 11904}
				}`), Valid: true},
				{RawMessage: json.RawMessage(`{
					"cache_creation_input_tokens": 500,
					"cache_read_input_tokens": 10000
				}`), Valid: true},
			},
			want: map[string]any{
				"cache_creation_input_tokens":        int64(500),
				"cache_read_input_tokens":            int64(33490),
				"input_tokens_details.cached_tokens": int64(11904),
			},
		},
		{
			name: "skips_string_boolean_null_values",
			rows: []pqtype.NullRawMessage{
				{RawMessage: json.RawMessage(`{"tokens":10,"name":"test","enabled":true,"nothing":null}`), Valid: true},
			},
			want: map[string]any{"tokens": int64(10)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tokens := make([]database.AIBridgeTokenUsage, 0, len(tc.rows))
			for _, metadata := range tc.rows {
				tokens = append(tokens, database.AIBridgeTokenUsage{ID: uuid.New(), Metadata: metadata})
			}
			require.Equal(t, tc.want, aggregateTokenMetadata(tokens))
		})
	}
}

func TestAggregateTokenUsage(t *testing.T) {
	t.Parallel()

	t.Run("empty_input", func(t *testing.T) {
		t.Parallel()
		result := aggregateTokenUsage(nil)
		require.Equal(t, int64(0), result.InputTokens)
		require.Equal(t, int64(0), result.OutputTokens)
		require.Empty(t, result.Metadata)
	})

	t.Run("sums_tokens_and_metadata", func(t *testing.T) {
		t.Parallel()
		tokens := []database.AIBridgeTokenUsage{
			{
				ID:           uuid.New(),
				InputTokens:  100,
				OutputTokens: 50,
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"reasoning_tokens":20}`),
					Valid:      true,
				},
			},
			{
				ID:           uuid.New(),
				InputTokens:  200,
				OutputTokens: 75,
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"reasoning_tokens":30}`),
					Valid:      true,
				},
			},
		}

		result := aggregateTokenUsage(tokens)
		require.Equal(t, int64(300), result.InputTokens)
		require.Equal(t, int64(125), result.OutputTokens)
		require.Equal(t, int64(50), result.Metadata["reasoning_tokens"])
	})

	t.Run("handles_rows_without_metadata", func(t *testing.T) {
		t.Parallel()
		tokens := []database.AIBridgeTokenUsage{
			{
				ID:           uuid.New(),
				InputTokens:  500,
				OutputTokens: 200,
				Metadata:     pqtype.NullRawMessage{Valid: false},
			},
		}

		result := aggregateTokenUsage(tokens)
		require.Equal(t, int64(500), result.InputTokens)
		require.Equal(t, int64(200), result.OutputTokens)
		require.Empty(t, result.Metadata)
	})
}

func TestSanitizeCredentialHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"valid_short", "s...t", "s...t"},
		{"valid_long", "sk-a...efgh", "sk-a...efgh"},
		{"valid_only_dots", "...", "..."},
		{"empty", "", ""},
		{"short_unmasked_secret", "abc12", "..."},
		{"missing_dots", "sk-abcdefgh", "..."},
		{"too_long", "sk-a...efghijklmn", "..."},
		{"raw_secret", "sk-proj-abc123xyz789", "..."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, sanitizeCredentialHint(tc.input))
		})
	}
}
