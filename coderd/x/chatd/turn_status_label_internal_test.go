package chatd

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
)

func TestNormalizeTurnStatusLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{
			name:  "accepts short label",
			input: "Finished tests",
			want:  "Finished tests",
			ok:    true,
		},
		{
			name:  "trims quotes and trailing punctuation",
			input: `"Submitted PR."`,
			want:  "Submitted PR",
			ok:    true,
		},
		{
			name:  "keeps version punctuation",
			input: "Updated v2.1 config",
			want:  "Updated v2.1 config",
			ok:    true,
		},
		{
			name:  "accepts five word label",
			input: "Updated workspace proxy routing rules",
			want:  "Updated workspace proxy routing rules",
			ok:    true,
		},
		{
			name:  "rejects agent phrasing",
			input: "Agent identified failing tests",
			ok:    false,
		},
		{
			name:  "rejects chat phrasing",
			input: "The chat is waiting now",
			ok:    false,
		},
		{
			name:  "rejects multiline labels",
			input: "Fixed bug\nAdded tests",
			ok:    false,
		},
		{
			name:  "rejects multi sentence labels",
			input: "Fixed bug. Added tests",
			ok:    false,
		},
		{
			name:  "rejects long labels",
			input: "Fixed the bug and added tests",
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := normalizeTurnStatusLabel(tt.input)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFallbackTurnStatusLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status database.ChatStatus
		want   string
	}{
		{status: database.ChatStatusWaiting, want: "Finished latest turn"},
		{status: database.ChatStatusPending, want: "Still working on request"},
		{status: database.ChatStatusRequiresAction, want: "Waiting for user input"},
		{status: database.ChatStatusError, want: "Hit an error"},
		{status: database.ChatStatus("unknown"), want: "Updated chat status"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, fallbackTurnStatusLabel(tt.status))
		})
	}
}

func TestDeriveTurnStatusLabelSourceSelection(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat := database.Chat{ID: uuid.New(), OwnerID: uuid.New(), Title: "status label"}

	t.Run("requires action is forced", func(t *testing.T) {
		t.Parallel()

		var generateCalls atomic.Int32
		result := (&Server{}).deriveTurnStatusLabel(ctx, chat, database.ChatStatusRequiresAction, runChatResult{
			FinalAssistantText: "Need input.",
			StatusLabelModel:   countingStatusLabelModel(&generateCalls, "Submitted PR"),
		}, logger)
		require.Equal(t, turnStatusLabelResult{
			Label:  "Waiting for user input",
			Source: turnStatusLabelSourceForced,
		}, result)
		require.Equal(t, int32(0), generateCalls.Load())
	})

	t.Run("pending is forced", func(t *testing.T) {
		t.Parallel()

		var generateCalls atomic.Int32
		result := (&Server{}).deriveTurnStatusLabel(ctx, chat, database.ChatStatusPending, runChatResult{
			FinalAssistantText: "Continuing.",
			StatusLabelModel:   countingStatusLabelModel(&generateCalls, "Submitted PR"),
		}, logger)
		require.Equal(t, turnStatusLabelResult{
			Label:  "Still working on request",
			Source: turnStatusLabelSourceForced,
		}, result)
		require.Equal(t, int32(0), generateCalls.Load())
	})

	t.Run("tool signal avoids model", func(t *testing.T) {
		t.Parallel()

		var generateCalls atomic.Int32
		result := (&Server{}).deriveTurnStatusLabel(ctx, chat, database.ChatStatusWaiting, runChatResult{
			FinalAssistantText: "Files are updated.",
			StatusLabelModel:   countingStatusLabelModel(&generateCalls, "Submitted PR"),
			StatusSignals: []turnStatusSignal{{
				Label:      "Updated files",
				Source:     turnStatusLabelSourceTool,
				Success:    true,
				Confidence: 70,
			}},
		}, logger)
		require.Equal(t, turnStatusLabelResult{
			Label:  "Updated files",
			Source: turnStatusLabelSourceTool,
		}, result)
		require.Equal(t, int32(0), generateCalls.Load())
	})

	t.Run("heuristic signal avoids model", func(t *testing.T) {
		t.Parallel()

		var generateCalls atomic.Int32
		result := (&Server{}).deriveTurnStatusLabel(ctx, chat, database.ChatStatusWaiting, runChatResult{
			FinalAssistantText: "Tests passed.",
			StatusLabelModel:   countingStatusLabelModel(&generateCalls, "Submitted PR"),
			StatusSignals: []turnStatusSignal{{
				Label:      "Finished tests",
				Source:     turnStatusLabelSourceHeuristic,
				Success:    true,
				Confidence: 100,
			}},
		}, logger)
		require.Equal(t, turnStatusLabelResult{
			Label:  "Finished tests",
			Source: turnStatusLabelSourceHeuristic,
		}, result)
		require.Equal(t, int32(0), generateCalls.Load())
	})

	t.Run("empty assistant text falls back", func(t *testing.T) {
		t.Parallel()

		var generateCalls atomic.Int32
		result := (&Server{}).deriveTurnStatusLabel(ctx, chat, database.ChatStatusWaiting, runChatResult{
			FinalAssistantText: "   ",
			StatusLabelModel:   countingStatusLabelModel(&generateCalls, "Submitted PR"),
		}, logger)
		require.Equal(t, turnStatusLabelResult{
			Label:  "Finished latest turn",
			Source: turnStatusLabelSourceFallback,
		}, result)
		require.Equal(t, int32(0), generateCalls.Load())
	})

	t.Run("nil status label model falls back", func(t *testing.T) {
		t.Parallel()

		result := (&Server{}).deriveTurnStatusLabel(ctx, chat, database.ChatStatusWaiting, runChatResult{
			FinalAssistantText: "I fixed the API bug.",
		}, logger)
		require.Equal(t, turnStatusLabelResult{
			Label:  "Finished latest turn",
			Source: turnStatusLabelSourceFallback,
		}, result)
	})

	t.Run("ambiguous turn uses model", func(t *testing.T) {
		t.Parallel()

		var generateCalls atomic.Int32
		result := (&Server{}).deriveTurnStatusLabel(ctx, chat, database.ChatStatusWaiting, runChatResult{
			FinalAssistantText: "I fixed the API bug.",
			StatusLabelModel:   countingStatusLabelModel(&generateCalls, "Fixed API bug"),
		}, logger)
		require.Equal(t, turnStatusLabelResult{
			Label:  "Fixed API bug",
			Source: turnStatusLabelSourceLLM,
		}, result)
		require.Equal(t, int32(1), generateCalls.Load())
	})

	t.Run("invalid model output falls back", func(t *testing.T) {
		t.Parallel()

		var generateCalls atomic.Int32
		result := (&Server{}).deriveTurnStatusLabel(ctx, chat, database.ChatStatusWaiting, runChatResult{
			FinalAssistantText: "I found failing tests.",
			StatusLabelModel:   countingStatusLabelModel(&generateCalls, "Agent identified failing tests"),
		}, logger)
		require.Equal(t, turnStatusLabelResult{
			Label:  "Finished latest turn",
			Source: turnStatusLabelSourceFallback,
		}, result)
		require.Equal(t, int32(1), generateCalls.Load())
	})
}

func TestTurnStatusSignalsFromContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content []fantasy.Content
		want    []turnStatusSignal
	}{
		{
			name:    "successful tests",
			content: successfulExecuteToolContent(t, "call-test", "go test ./coderd/x/chatd"),
			want: []turnStatusSignal{{
				Label:      "Finished tests",
				Source:     turnStatusLabelSourceHeuristic,
				Success:    true,
				Confidence: 100,
			}},
		},
		{
			name:    "failing tests",
			content: failedExecuteToolContent(t, "call-test", "pytest"),
			want: []turnStatusSignal{{
				Label:      "Tests failing",
				Source:     turnStatusLabelSourceHeuristic,
				Success:    false,
				Confidence: 100,
			}},
		},
		{
			name:    "created pr",
			content: successfulExecuteToolContent(t, "call-pr", "gh pr create --fill"),
			want: []turnStatusSignal{{
				Label:      "Submitted PR",
				Source:     turnStatusLabelSourceHeuristic,
				Success:    true,
				Confidence: 100,
			}},
		},
		{
			name:    "created pr after tests",
			content: successfulExecuteToolContent(t, "call-pr", "go test ./... && gh pr create --fill"),
			want: []turnStatusSignal{{
				Label:      "Submitted PR",
				Source:     turnStatusLabelSourceHeuristic,
				Success:    true,
				Confidence: 100,
			}},
		},
		{
			name:    "created commit",
			content: successfulExecuteToolContent(t, "call-commit", "git commit -m change"),
			want: []turnStatusSignal{{
				Label:      "Created commit",
				Source:     turnStatusLabelSourceHeuristic,
				Success:    true,
				Confidence: 100,
			}},
		},
		{
			name:    "failed pr create ignored",
			content: failedExecuteToolContent(t, "call-pr", "gh pr create --fill"),
		},
		{
			name:    "failed compound test and pr ignored",
			content: failedExecuteToolContent(t, "call-pr", "go test ./... && gh pr create --fill"),
		},
		{
			name:    "failed commit ignored",
			content: failedExecuteToolContent(t, "call-commit", "git commit -m change"),
		},
		{
			name:    "unrecognized execute command ignored",
			content: successfulExecuteToolContent(t, "call-ls", "ls -la"),
		},
		{
			name: "updated files",
			content: []fantasy.Content{
				toolResultContent(t, "call-edit", "edit_files", map[string]any{"ok": true}),
			},
			want: []turnStatusSignal{{
				Label:      "Updated files",
				Source:     turnStatusLabelSourceTool,
				Success:    true,
				Confidence: 70,
			}},
		},
		{
			name: "wrote file",
			content: []fantasy.Content{
				toolResultContent(t, "call-write", "write_file", map[string]any{"ok": true}),
			},
			want: []turnStatusSignal{{
				Label:      "Updated files",
				Source:     turnStatusLabelSourceTool,
				Success:    true,
				Confidence: 70,
			}},
		},
		{
			name: "failed file update ignored",
			content: []fantasy.Content{
				toolResultContent(t, "call-edit", "edit_files", map[string]any{"ok": false}),
			},
		},
		{
			name: "provider executed result is ignored",
			content: []fantasy.Content{
				func() fantasy.ToolResultContent {
					result := toolResultContent(t, "call-edit", "edit_files", map[string]any{"ok": true})
					result.ProviderExecuted = true
					return result
				}(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, turnStatusSignalsFromContent(tt.content))
		})
	}
}

func TestSelectTurnStatusSignalUsesLatestEqualConfidence(t *testing.T) {
	t.Parallel()

	signal, ok := selectTurnStatusSignal([]turnStatusSignal{
		{
			Label:      "Submitted PR",
			Source:     turnStatusLabelSourceHeuristic,
			Success:    true,
			Confidence: 100,
		},
		{
			Label:      "Finished tests",
			Source:     turnStatusLabelSourceHeuristic,
			Success:    true,
			Confidence: 100,
		},
	})
	require.True(t, ok)
	require.Equal(t, turnStatusSignal{
		Label:      "Finished tests",
		Source:     turnStatusLabelSourceHeuristic,
		Success:    true,
		Confidence: 100,
	}, signal)
}

func TestSelectTurnStatusSignalFiltersLowConfidence(t *testing.T) {
	t.Parallel()

	_, ok := selectTurnStatusSignal([]turnStatusSignal{{
		Label:      "Updated files",
		Source:     turnStatusLabelSourceTool,
		Success:    true,
		Confidence: 69,
	}})
	require.False(t, ok)
}

func TestIsTestCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{name: "go test", command: "go test ./...", want: true},
		{name: "pnpm test", command: "pnpm test", want: true},
		{name: "pnpm run test", command: "pnpm run test", want: true},
		{name: "npm run test", command: "npm run test", want: true},
		{name: "npm test", command: "npm test", want: true},
		{name: "pytest", command: "pytest -q", want: true},
		{name: "vitest", command: "vitest run", want: true},
		{name: "cargo test", command: "cargo test", want: true},
		{name: "make test", command: "make test", want: true},
		{name: "yarn test", command: "yarn test", want: true},
		{name: "yarn run test", command: "yarn run test", want: true},
		{name: "after shell separator", command: "cd site && pnpm test", want: true},
		{name: "after compact semicolon", command: "cd site; go test ./...", want: true},
		{name: "after pipe", command: "cat config.json | go test ./...", want: true},
		{name: "after environment assignment", command: "VERBOSE=1 go test ./...", want: true},
		{name: "pytest as argument", command: "pip install pytest", want: false},
		{name: "test command after non-assignment", command: "echo foo=bar go test", want: false},
		{name: "flag assignments before command", command: "--format=json --output=file go test", want: false},
		{name: "make testing", command: "make testing", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, isTestCommand(tt.command))
		})
	}
}

func successfulExecuteToolContent(t *testing.T, callID string, command string) []fantasy.Content {
	t.Helper()
	return executeToolContent(t, callID, command, map[string]any{
		"success":   true,
		"exit_code": 0,
	})
}

func failedExecuteToolContent(t *testing.T, callID string, command string) []fantasy.Content {
	t.Helper()
	return executeToolContent(t, callID, command, map[string]any{
		"success":   false,
		"exit_code": 1,
	})
}

func executeToolContent(t *testing.T, callID string, command string, payload map[string]any) []fantasy.Content {
	t.Helper()

	executeArgs, err := json.Marshal(map[string]string{"command": command})
	require.NoError(t, err)
	return []fantasy.Content{
		fantasy.ToolCallContent{
			ToolCallID: callID,
			ToolName:   "execute",
			Input:      string(executeArgs),
		},
		toolResultContent(t, callID, "execute", payload),
	}
}

func toolResultContent(t *testing.T, callID string, toolName string, payload map[string]any) fantasy.ToolResultContent {
	t.Helper()

	result, err := json.Marshal(payload)
	require.NoError(t, err)
	return fantasy.ToolResultContent{
		ToolCallID: callID,
		ToolName:   toolName,
		Result:     fantasy.ToolResultOutputContentText{Text: string(result)},
	}
}

func countingStatusLabelModel(calls *atomic.Int32, label string) fantasy.LanguageModel {
	return &chattest.FakeModel{
		ProviderName: "openai",
		ModelName:    "test-model",
		GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
			calls.Add(1)
			return &fantasy.Response{
				Content: fantasy.ResponseContent{
					fantasy.TextContent{Text: label},
				},
			}, nil
		},
	}
}
