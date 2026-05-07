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

func TestNormalizePushStatusLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{
			name:  "accepts short label",
			input: "Finished unit tests",
			want:  "Finished unit tests",
			ok:    true,
		},
		{
			name:  "trims quotes and trailing punctuation",
			input: `"Submitted PR."`,
			want:  "Submitted PR",
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
			name:  "rejects long labels",
			input: "Fixed the bug and added tests",
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := normalizePushStatusLabel(tt.input)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFallbackPushStatusLabel(t *testing.T) {
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
			require.Equal(t, tt.want, fallbackPushStatusLabel(tt.status))
		})
	}
}

func TestDeriveTurnStatusLabelSourceSelection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat := database.Chat{ID: uuid.New(), OwnerID: uuid.New(), Title: "status label"}

	t.Run("requires action is forced", func(t *testing.T) {
		t.Parallel()

		var generateCalls atomic.Int32
		result := (&Server{}).deriveTurnStatusLabel(ctx, chat, database.ChatStatusRequiresAction, runChatResult{
			FinalAssistantText: "Need input.",
			PushSummaryModel:   countingStatusLabelModel(&generateCalls, "Submitted PR"),
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
			PushSummaryModel:   countingStatusLabelModel(&generateCalls, "Submitted PR"),
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
			PushSummaryModel:   countingStatusLabelModel(&generateCalls, "Submitted PR"),
			StatusSignals: []TurnStatusSignal{{
				Label:      "Updated files",
				Category:   string(turnStatusLabelSourceTool),
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
			PushSummaryModel:   countingStatusLabelModel(&generateCalls, "Submitted PR"),
			StatusSignals: []TurnStatusSignal{{
				Label:      "Finished unit tests",
				Category:   string(turnStatusLabelSourceHeuristic),
				Success:    true,
				Confidence: 100,
			}},
		}, logger)
		require.Equal(t, turnStatusLabelResult{
			Label:  "Finished unit tests",
			Source: turnStatusLabelSourceHeuristic,
		}, result)
		require.Equal(t, int32(0), generateCalls.Load())
	})

	t.Run("ambiguous turn uses model", func(t *testing.T) {
		t.Parallel()

		var generateCalls atomic.Int32
		result := (&Server{}).deriveTurnStatusLabel(ctx, chat, database.ChatStatusWaiting, runChatResult{
			FinalAssistantText: "I fixed the API bug.",
			PushSummaryModel:   countingStatusLabelModel(&generateCalls, "Fixed API bug"),
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
			PushSummaryModel:   countingStatusLabelModel(&generateCalls, "Agent identified failing tests"),
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

	executeArgs, err := json.Marshal(map[string]string{"command": "go test ./coderd/x/chatd"})
	require.NoError(t, err)
	executeResult, err := json.Marshal(map[string]any{"success": true, "exit_code": 0})
	require.NoError(t, err)
	editResult, err := json.Marshal(map[string]any{"ok": true})
	require.NoError(t, err)

	signals := turnStatusSignalsFromContent([]fantasy.Content{
		fantasy.ToolCallContent{
			ToolCallID: "call-test",
			ToolName:   "execute",
			Input:      string(executeArgs),
		},
		fantasy.ToolResultContent{
			ToolCallID: "call-test",
			ToolName:   "execute",
			Result:     fantasy.ToolResultOutputContentText{Text: string(executeResult)},
		},
		fantasy.ToolResultContent{
			ToolCallID: "call-edit",
			ToolName:   "edit_files",
			Result:     fantasy.ToolResultOutputContentText{Text: string(editResult)},
		},
	})

	require.Equal(t, []TurnStatusSignal{
		{
			Label:      "Finished unit tests",
			Category:   string(turnStatusLabelSourceHeuristic),
			Success:    true,
			Confidence: 100,
		},
		{
			Label:      "Updated files",
			Category:   string(turnStatusLabelSourceTool),
			Success:    true,
			Confidence: 70,
		},
	}, signals)
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
