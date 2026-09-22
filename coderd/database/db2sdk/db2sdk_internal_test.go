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
		want := codersdk.AIBridgeAttribution{
			"workspace_id": wsID.String(),
		}
		require.Equal(t, want, aiBridgeInterceptionAttribution(intc))
	})
}

// TestBuildAIBridgeThreadAttributionAndActions verifies that attribution comes
// from the root only and that non-root interceptions become actions, including
// tool-less interceptions.
func TestBuildAIBridgeThreadAttributionAndActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rootWorkspace uuid.NullUUID
		wantRoot      codersdk.AIBridgeAttribution
	}{
		{
			name: "known_root",
			rootWorkspace: uuid.NullUUID{
				UUID:  uuid.MustParse("11111111-1111-1111-1111-111111111111"),
				Valid: true,
			},
			wantRoot: codersdk.AIBridgeAttribution{
				"workspace_id": "11111111-1111-1111-1111-111111111111",
			},
		},
		{
			name:     "unknown_root",
			wantRoot: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			threadID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
			childWithWorkspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
			toolLessChildID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
			childWorkspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
			rootTools := []database.AIBridgeToolUsage{
				{
					ID:             uuid.MustParse("00000000-0000-0000-0000-000000000010"),
					InterceptionID: threadID,
					Tool:           "root_tool",
				},
			}
			interceptions := []database.AIBridgeInterception{
				{
					// The root ID must match threadID. Attribution on children must
					// not replace this value.
					ID:          threadID,
					Model:       "root-model",
					Provider:    "openai",
					WorkspaceID: tt.rootWorkspace,
				},
				{
					ID:          childWithWorkspaceID,
					Model:       "child-model",
					Provider:    "openai",
					WorkspaceID: uuid.NullUUID{UUID: childWorkspaceID, Valid: true},
				},
				{
					ID:       toolLessChildID,
					Model:    "tool-less-child-model",
					Provider: "openai",
				},
			}

			thread := buildAIBridgeThread(
				threadID,
				interceptions,
				make(map[uuid.UUID][]database.AIBridgeTokenUsage),
				map[uuid.UUID][]database.AIBridgeToolUsage{
					threadID: rootTools,
				},
				make(map[uuid.UUID][]database.AIBridgeUserPrompt),
				make(map[uuid.UUID][]database.AIBridgeModelThought),
			)

			require.Equal(t, tt.wantRoot, thread.Attribution)
			require.Len(t, thread.AgenticActions, 3)

			rootAction := thread.AgenticActions[0]
			require.Equal(t, threadID, rootAction.InterceptionID)
			require.Equal(t, tt.wantRoot, rootAction.Attribution)
			require.Len(t, rootAction.ToolCalls, 1)
			require.Equal(t, "root_tool", rootAction.ToolCalls[0].Tool)

			childAction := thread.AgenticActions[1]
			require.Equal(t, childWithWorkspaceID, childAction.InterceptionID)
			want := codersdk.AIBridgeAttribution{
				"workspace_id": childWorkspaceID.String(),
			}
			require.Equal(t, want, childAction.Attribution)
			require.Empty(t, childAction.ToolCalls)
			require.NotNil(t, childAction.ToolCalls)

			toolLessChildAction := thread.AgenticActions[2]
			require.Equal(t, toolLessChildID, toolLessChildAction.InterceptionID)
			require.Empty(t, toolLessChildAction.Attribution)
			require.Empty(t, toolLessChildAction.ToolCalls)
			require.NotNil(t, toolLessChildAction.ToolCalls)

			var raw map[string]json.RawMessage
			jsonBytes, err := json.Marshal(thread)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(jsonBytes, &raw))
			require.NotContains(t, raw, "interception_attributions")
			wantRootJSON, err := json.Marshal(tt.wantRoot)
			require.NoError(t, err)
			require.JSONEq(t, string(wantRootJSON), string(raw["attribution"]))
			if tt.wantRoot == nil {
				require.JSONEq(t, `{}`, string(raw["attribution"]))
			}

			var rawActions []struct {
				Attribution json.RawMessage `json:"attribution"`
			}
			require.NoError(t, json.Unmarshal(raw["agentic_actions"], &rawActions))
			require.JSONEq(t, `{}`, string(rawActions[2].Attribution))
			require.NotContains(t, string(raw["attribution"]), childWithWorkspaceID.String())
			require.NotContains(t, string(raw["attribution"]), toolLessChildID.String())
		})
	}
}

func TestBuildAIBridgeThreadMissingRoot(t *testing.T) {
	t.Parallel()

	threadID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	childID := uuid.MustParse("00000000-0000-0000-0000-000000000102")
	childWorkspaceID := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	thread := buildAIBridgeThread(
		threadID,
		[]database.AIBridgeInterception{
			{
				// This is the fallback interception, not the actual root.
				ID:          childID,
				Model:       "child-model",
				Provider:    "openai",
				WorkspaceID: uuid.NullUUID{UUID: childWorkspaceID, Valid: true},
			},
		},
		make(map[uuid.UUID][]database.AIBridgeTokenUsage),
		make(map[uuid.UUID][]database.AIBridgeToolUsage),
		make(map[uuid.UUID][]database.AIBridgeUserPrompt),
		make(map[uuid.UUID][]database.AIBridgeModelThought),
	)

	require.Empty(t, thread.Attribution)
	require.Len(t, thread.AgenticActions, 1)
	require.Equal(t, childID, thread.AgenticActions[0].InterceptionID)
	want := codersdk.AIBridgeAttribution{
		"workspace_id": childWorkspaceID.String(),
	}
	require.Equal(t, want, thread.AgenticActions[0].Attribution)
	require.Empty(t, thread.AgenticActions[0].ToolCalls)
	require.NotNil(t, thread.AgenticActions[0].ToolCalls)

	var raw struct {
		Attribution    json.RawMessage `json:"attribution"`
		AgenticActions []struct {
			Attribution json.RawMessage `json:"attribution"`
		} `json:"agentic_actions"`
	}
	jsonBytes, err := json.Marshal(thread)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(jsonBytes, &raw))
	require.JSONEq(t, `{}`, string(raw.Attribution))
}

func TestBuildAIBridgeThreadStandaloneRootActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		workspaceID     uuid.NullUUID
		wantAttribution codersdk.AIBridgeAttribution
	}{
		{
			name: "known_root",
			workspaceID: uuid.NullUUID{
				UUID:  uuid.MustParse("44444444-4444-4444-4444-444444444444"),
				Valid: true,
			},
			wantAttribution: codersdk.AIBridgeAttribution{
				"workspace_id": "44444444-4444-4444-4444-444444444444",
			},
		},
		{name: "unknown_root"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			threadID := uuid.MustParse("00000000-0000-0000-0000-000000000201")
			thread := buildAIBridgeThread(
				threadID,
				[]database.AIBridgeInterception{{
					ID:          threadID,
					Model:       "root-model",
					Provider:    "openai",
					WorkspaceID: tt.workspaceID,
				}},
				make(map[uuid.UUID][]database.AIBridgeTokenUsage),
				make(map[uuid.UUID][]database.AIBridgeToolUsage),
				make(map[uuid.UUID][]database.AIBridgeUserPrompt),
				make(map[uuid.UUID][]database.AIBridgeModelThought),
			)

			require.Equal(t, tt.wantAttribution, thread.Attribution)
			require.Empty(t, thread.AgenticActions)
			require.NotNil(t, thread.AgenticActions)

			var raw struct {
				Attribution json.RawMessage `json:"attribution"`
			}
			jsonBytes, err := json.Marshal(thread)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(jsonBytes, &raw))
			if tt.wantAttribution == nil {
				require.JSONEq(t, `{}`, string(raw.Attribution))
			} else {
				wantJSON, err := json.Marshal(tt.wantAttribution)
				require.NoError(t, err)
				require.JSONEq(t, string(wantJSON), string(raw.Attribution))
			}
		})
	}
}
