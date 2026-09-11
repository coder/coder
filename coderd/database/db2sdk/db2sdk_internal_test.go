package db2sdk

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func TestAggregateTokenMetadata(t *testing.T) {
	t.Parallel()

	t.Run("empty_input", func(t *testing.T) {
		t.Parallel()
		result := aggregateTokenMetadata(nil)
		require.Empty(t, result)
	})

	t.Run("sums_across_rows", func(t *testing.T) {
		t.Parallel()
		tokens := []database.AIBridgeTokenUsage{
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"cache_read_tokens":100,"reasoning_tokens":50}`),
					Valid:      true,
				},
			},
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"cache_read_tokens":200,"reasoning_tokens":75}`),
					Valid:      true,
				},
			},
		}

		result := aggregateTokenMetadata(tokens)
		require.Equal(t, int64(300), result["cache_read_tokens"])
		require.Equal(t, int64(125), result["reasoning_tokens"])
		require.Len(t, result, 2)
	})

	t.Run("skips_null_and_invalid_metadata", func(t *testing.T) {
		t.Parallel()
		tokens := []database.AIBridgeTokenUsage{
			{
				ID:       uuid.New(),
				Metadata: pqtype.NullRawMessage{Valid: false},
			},
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: nil,
					Valid:      true,
				},
			},
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"tokens":42}`),
					Valid:      true,
				},
			},
		}

		result := aggregateTokenMetadata(tokens)
		require.Equal(t, int64(42), result["tokens"])
		require.Len(t, result, 1)
	})

	t.Run("skips_non_integer_values", func(t *testing.T) {
		t.Parallel()
		tokens := []database.AIBridgeTokenUsage{
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					// Float values fail json.Number.Int64(), so they
					// are silently dropped.
					RawMessage: json.RawMessage(`{"good":10,"fractional":1.5}`),
					Valid:      true,
				},
			},
		}

		result := aggregateTokenMetadata(tokens)
		require.Equal(t, int64(10), result["good"])
		_, hasFractional := result["fractional"]
		require.False(t, hasFractional)
	})

	t.Run("skips_malformed_json", func(t *testing.T) {
		t.Parallel()
		tokens := []database.AIBridgeTokenUsage{
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`not json`),
					Valid:      true,
				},
			},
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"tokens":5}`),
					Valid:      true,
				},
			},
		}

		result := aggregateTokenMetadata(tokens)
		// The malformed row is skipped, the valid one is counted.
		require.Equal(t, int64(5), result["tokens"])
		require.Len(t, result, 1)
	})

	t.Run("flattens_nested_objects", func(t *testing.T) {
		t.Parallel()
		tokens := []database.AIBridgeTokenUsage{
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{
						"cache_read_tokens": 100,
						"cache": {"creation_tokens": 40, "read_tokens": 60},
						"reasoning_tokens": 50,
						"tags": ["a", "b"]
					}`),
					Valid: true,
				},
			},
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{
						"cache_read_tokens": 200,
						"cache": {"creation_tokens": 10}
					}`),
					Valid: true,
				},
			},
		}

		result := aggregateTokenMetadata(tokens)
		require.Equal(t, int64(300), result["cache_read_tokens"])
		require.Equal(t, int64(50), result["reasoning_tokens"])
		require.Equal(t, int64(50), result["cache.creation_tokens"])
		require.Equal(t, int64(60), result["cache.read_tokens"])
		// Arrays are skipped.
		_, hasTags := result["tags"]
		require.False(t, hasTags)
		require.Len(t, result, 4)
	})

	t.Run("flattens_deeply_nested_objects", func(t *testing.T) {
		t.Parallel()
		tokens := []database.AIBridgeTokenUsage{
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{
						"provider": {
							"anthropic": {"cache_creation_tokens": 100, "cache_read_tokens": 200},
							"openai": {"reasoning_tokens": 50}
						},
						"total": 500
					}`),
					Valid: true,
				},
			},
		}

		result := aggregateTokenMetadata(tokens)
		require.Equal(t, int64(100), result["provider.anthropic.cache_creation_tokens"])
		require.Equal(t, int64(200), result["provider.anthropic.cache_read_tokens"])
		require.Equal(t, int64(50), result["provider.openai.reasoning_tokens"])
		require.Equal(t, int64(500), result["total"])
		require.Len(t, result, 4)
	})

	// Real-world provider metadata shapes from
	// https://github.com/coder/aibridge/issues/150.
	t.Run("aggregates_real_provider_metadata", func(t *testing.T) {
		t.Parallel()
		tokens := []database.AIBridgeTokenUsage{
			{
				// Anthropic-style: cache fields are top-level.
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{
						"cache_creation_input_tokens": 0,
						"cache_read_input_tokens": 23490
					}`),
					Valid: true,
				},
			},
			{
				// OpenAI-style: cache fields are nested inside
				// input_tokens_details.
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{
						"input_tokens_details": {"cached_tokens": 11904}
					}`),
					Valid: true,
				},
			},
			{
				// Second Anthropic row to verify summing.
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{
						"cache_creation_input_tokens": 500,
						"cache_read_input_tokens": 10000
					}`),
					Valid: true,
				},
			},
		}

		result := aggregateTokenMetadata(tokens)
		// Anthropic fields are summed across two rows.
		require.Equal(t, int64(500), result["cache_creation_input_tokens"])
		require.Equal(t, int64(33490), result["cache_read_input_tokens"])
		// OpenAI nested field is flattened with dot notation.
		require.Equal(t, int64(11904), result["input_tokens_details.cached_tokens"])
		require.Len(t, result, 3)
	})

	t.Run("skips_string_boolean_null_values", func(t *testing.T) {
		t.Parallel()
		tokens := []database.AIBridgeTokenUsage{
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"tokens":10,"name":"test","enabled":true,"nothing":null}`),
					Valid:      true,
				},
			},
		}

		result := aggregateTokenMetadata(tokens)
		require.Equal(t, int64(10), result["tokens"])
		require.Len(t, result, 1)
	})
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

// TestAIBridgeInterceptionAttribution covers unknown and workspace attribution.
func TestAIBridgeInterceptionAttribution(t *testing.T) {
	t.Parallel()

	t.Run("nil_when_no_workspace", func(t *testing.T) {
		t.Parallel()
		intc := database.AIBridgeInterception{
			ID: uuid.New(),
			// WorkspaceID is zero-value (not valid).
		}
		require.Nil(t, aiBridgeInterceptionAttribution(intc))
	})

	t.Run("workspace_id_only", func(t *testing.T) {
		t.Parallel()
		wsID := uuid.New()
		intc := database.AIBridgeInterception{
			ID:          uuid.New(),
			WorkspaceID: uuid.NullUUID{UUID: wsID, Valid: true},
		}
		got := aiBridgeInterceptionAttribution(intc)
		require.Equal(t, map[string]string{
			"workspace_id": wsID.String(),
		}, got)
	})
}

// TestBuildAIBridgeThreadInterceptions verifies that every interception,
// including tool-less rows, appears in the map keyed by interception ID string,
// with nil attribution for unknown-workspace rows.
func TestBuildAIBridgeThreadInterceptions(t *testing.T) {
	t.Parallel()

	threadID := uuid.New()
	i1 := uuid.New()
	i2 := uuid.New()
	wsID := uuid.New()

	interceptions := []database.AIBridgeInterception{
		{
			// Root interception, tool-less, no attribution.
			ID:       i1,
			Model:    "gpt-4",
			Provider: "openai",
		},
		{
			// Agentic child with workspace attribution.
			ID:          i2,
			Model:       "gpt-4",
			Provider:    "openai",
			WorkspaceID: uuid.NullUUID{UUID: wsID, Valid: true},
		},
	}

	thread := buildAIBridgeThread(
		threadID,
		interceptions,
		make(map[uuid.UUID][]database.AIBridgeTokenUsage),
		make(map[uuid.UUID][]database.AIBridgeToolUsage),
		make(map[uuid.UUID][]database.AIBridgeUserPrompt),
		make(map[uuid.UUID][]database.AIBridgeModelThought),
	)

	// Both interceptions must appear in the map.
	require.Len(t, thread.InterceptionAttributions, 2)

	// First: no attribution (workspace unknown) -> nil inner map.
	attrib1, ok1 := thread.InterceptionAttributions[i1.String()]
	require.True(t, ok1, "interception i1 must be in map")
	require.Nil(t, attrib1)

	// Second: workspace attribution -> populated inner map.
	attrib2, ok2 := thread.InterceptionAttributions[i2.String()]
	require.True(t, ok2, "interception i2 must be in map")
	require.Equal(t, &codersdk.AIBridgeAttribution{
		"workspace_id": wsID.String(),
	}, attrib2)

	// Empty thread: outer map is non-nil and serializes as {}.
	emptyThread := buildAIBridgeThread(
		uuid.New(),
		nil,
		make(map[uuid.UUID][]database.AIBridgeTokenUsage),
		make(map[uuid.UUID][]database.AIBridgeToolUsage),
		make(map[uuid.UUID][]database.AIBridgeUserPrompt),
		make(map[uuid.UUID][]database.AIBridgeModelThought),
	)
	require.NotNil(t, emptyThread.InterceptionAttributions, "InterceptionAttributions must not be nil")
	require.IsType(t, map[string]*codersdk.AIBridgeAttribution{}, emptyThread.InterceptionAttributions)
	require.Empty(t, emptyThread.InterceptionAttributions)

	// Verify serialization: empty thread -> {} and nil attribution -> null.
	jsonBytes, err := json.Marshal(emptyThread.InterceptionAttributions)
	require.NoError(t, err)
	require.Equal(t, `{}`, string(jsonBytes))

	jsonBytes, err = json.Marshal(thread.InterceptionAttributions[i1.String()])
	require.NoError(t, err)
	require.Equal(t, `null`, string(jsonBytes))
}
