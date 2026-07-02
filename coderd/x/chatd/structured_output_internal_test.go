package chatd

import (
	"encoding/json"
	"testing"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/structuredoutput"
	"github.com/coder/coder/v2/codersdk"
)

const testResponseFormatSchema = `{
	"type": "object",
	"properties": {"answer": {"type": "string"}},
	"required": ["answer"]
}`

func testResponseFormat() codersdk.ChatResponseFormat {
	return codersdk.ChatResponseFormat{
		Type: codersdk.ChatResponseFormatTypeJSONSchema,
		JSONSchema: &codersdk.ChatResponseFormatJSONSchema{
			Name:   "test_answer",
			Schema: json.RawMessage(testResponseFormatSchema),
		},
	}
}

func chatMessageWithParts(t *testing.T, id int64, role database.ChatMessageRole, parts []codersdk.ChatMessagePart) database.ChatMessage {
	t.Helper()
	raw, err := json.Marshal(parts)
	require.NoError(t, err)
	return database.ChatMessage{
		ID:             id,
		Role:           role,
		Visibility:     database.ChatMessageVisibilityBoth,
		Content:        pqtype.NullRawMessage{RawMessage: raw, Valid: true},
		ContentVersion: chatprompt.CurrentContentVersion,
	}
}

func userMessageWithFormat(t *testing.T, id int64, text string, format *codersdk.ChatResponseFormat) database.ChatMessage {
	t.Helper()
	parts := []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)}
	if format != nil {
		parts = append(parts, codersdk.ChatMessageResponseFormat(*format))
	}
	return chatMessageWithParts(t, id, database.ChatMessageRoleUser, parts)
}

func textAssistantMessage(t *testing.T, id int64, text string) database.ChatMessage {
	t.Helper()
	return chatMessageWithParts(t, id, database.ChatMessageRoleAssistant, []codersdk.ChatMessagePart{
		codersdk.ChatMessageText(text),
	})
}

func TestActiveTurnResponseFormat(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := slog.Make()
	format := testResponseFormat()

	t.Run("NoFormat", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "hello", nil),
		}
		require.Nil(t, activeTurnResponseFormat(ctx, logger, messages))
	})

	t.Run("ActiveTurnFormat", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "hello", &format),
		}
		req := activeTurnResponseFormat(ctx, logger, messages)
		require.NotNil(t, req)
		require.Equal(t, "test_answer", req.Name)
	})

	t.Run("OlderTurnFormatIgnored", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured please", &format),
			textAssistantMessage(t, 2, "done"),
			userMessageWithFormat(t, 3, "now plain text", nil),
		}
		require.Nil(t, activeTurnResponseFormat(ctx, logger, messages))
	})

	t.Run("MultiTurnLatestWins", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "plain", nil),
			textAssistantMessage(t, 2, "ok"),
			userMessageWithFormat(t, 3, "structured", &format),
			textAssistantMessage(t, 4, "working on it"),
		}
		req := activeTurnResponseFormat(ctx, logger, messages)
		require.NotNil(t, req)
	})

	t.Run("SurvivesCompaction", func(t *testing.T) {
		t.Parallel()
		// Compaction marks older rows Compressed and inserts a
		// compressed summary user row; the trigger user message of
		// the active turn stays uncompressed.
		compressedSummary := userMessageWithFormat(t, 3, "summary", nil)
		compressedSummary.Visibility = database.ChatMessageVisibilityModel
		compressedSummary.Compressed = true
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "old", nil),
			textAssistantMessage(t, 2, "old answer"),
			compressedSummary,
			userMessageWithFormat(t, 4, "structured", &format),
			textAssistantMessage(t, 5, "step one"),
		}
		req := activeTurnResponseFormat(ctx, logger, messages)
		require.NotNil(t, req)
	})

	t.Run("SkipsModelVisibilityUserRows", func(t *testing.T) {
		t.Parallel()
		hidden := userMessageWithFormat(t, 2, "injected context", nil)
		hidden.Visibility = database.ChatMessageVisibilityModel
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
			hidden,
		}
		req := activeTurnResponseFormat(ctx, logger, messages)
		require.NotNil(t, req)
	})

	t.Run("LastPartWinsWithinMessage", func(t *testing.T) {
		t.Parallel()
		second := testResponseFormat()
		second.JSONSchema.Name = "second_format"
		msg := chatMessageWithParts(t, 1, database.ChatMessageRoleUser, []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("hello"),
			codersdk.ChatMessageResponseFormat(format),
			codersdk.ChatMessageResponseFormat(second),
		})
		req := activeTurnResponseFormat(ctx, logger, []database.ChatMessage{msg})
		require.NotNil(t, req)
		require.Equal(t, "second_format", req.Name)
	})

	t.Run("InvalidPersistedFormatIgnored", func(t *testing.T) {
		t.Parallel()
		invalid := codersdk.ChatResponseFormat{
			Type: codersdk.ChatResponseFormatTypeJSONSchema,
			JSONSchema: &codersdk.ChatResponseFormatJSONSchema{
				Name:   "bad",
				Schema: json.RawMessage(`{"type":"string"}`),
			},
		}
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &invalid),
		}
		require.Nil(t, activeTurnResponseFormat(ctx, logger, messages))
	})
}

func TestExtractStructuredOutputValue(t *testing.T) {
	t.Parallel()

	format := testResponseFormat()
	finalizerCall := func(id int64, callID string) database.ChatMessage {
		return chatMessageWithParts(t, id, database.ChatMessageRoleAssistant, []codersdk.ChatMessagePart{
			codersdk.ChatMessageToolCall(callID, structuredoutput.ToolName, json.RawMessage(`{"output":{"answer":"42"}}`)),
		})
	}
	finalizerResult := func(id int64, callID string, result string, isError bool) database.ChatMessage {
		return chatMessageWithParts(t, id, database.ChatMessageRoleTool, []codersdk.ChatMessagePart{
			codersdk.ChatMessageToolResult(callID, structuredoutput.ToolName, json.RawMessage(result), isError, false),
		})
	}

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
			finalizerCall(2, "call_1"),
			finalizerResult(3, "call_1", `{"answer":"42"}`, false),
		}
		value, ok, err := ExtractStructuredOutputValue(messages)
		require.NoError(t, err)
		require.True(t, ok)
		require.JSONEq(t, `{"answer":"42"}`, string(value))
	})

	t.Run("ErrorResultDoesNotCount", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
			finalizerCall(2, "call_1"),
			finalizerResult(3, "call_1", `{"error":"validation failed"}`, true),
		}
		_, ok, err := ExtractStructuredOutputValue(messages)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("PreviousTurnResultIgnored", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
			finalizerCall(2, "call_1"),
			finalizerResult(3, "call_1", `{"answer":"old"}`, false),
			userMessageWithFormat(t, 4, "again", &format),
			textAssistantMessage(t, 5, "working"),
		}
		_, ok, err := ExtractStructuredOutputValue(messages)
		require.NoError(t, err)
		require.False(t, ok)
	})
}

func TestDecideGenerationActionStructuredOutput(t *testing.T) {
	t.Parallel()

	format := testResponseFormat()
	stopAfter := map[string]struct{}{structuredoutput.ToolName: {}}

	t.Run("TextOnlyDoesNotFinish", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
			textAssistantMessage(t, 2, "here is your answer as text"),
		}
		decision, err := decideGenerationAction(generationDecisionInput{
			messages:                 messages,
			stopAfterTools:           stopAfter,
			structuredOutputRequired: true,
			maxSteps:                 10,
		})
		require.NoError(t, err)
		require.Equal(t, generationActionGenerateAssistant, decision.kind)
	})

	t.Run("TextOnlyFinishesWithoutStructuredOutput", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "plain", nil),
			textAssistantMessage(t, 2, "answer"),
		}
		decision, err := decideGenerationAction(generationDecisionInput{
			messages: messages,
			maxSteps: 10,
		})
		require.NoError(t, err)
		require.Equal(t, generationActionFinishTurn, decision.kind)
		require.Equal(t, generationFinishReasonComplete, decision.finishReason)
	})

	t.Run("SuccessfulFinalizerFinishesTurn", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
			chatMessageWithParts(t, 2, database.ChatMessageRoleAssistant, []codersdk.ChatMessagePart{
				codersdk.ChatMessageToolCall("call_1", structuredoutput.ToolName, json.RawMessage(`{"output":{"answer":"42"}}`)),
			}),
			chatMessageWithParts(t, 3, database.ChatMessageRoleTool, []codersdk.ChatMessagePart{
				codersdk.ChatMessageToolResult("call_1", structuredoutput.ToolName, json.RawMessage(`{"answer":"42"}`), false, false),
			}),
		}
		decision, err := decideGenerationAction(generationDecisionInput{
			messages:                 messages,
			stopAfterTools:           stopAfter,
			structuredOutputRequired: true,
			maxSteps:                 10,
		})
		require.NoError(t, err)
		require.Equal(t, generationActionFinishTurn, decision.kind)
		require.Equal(t, generationFinishReasonStopAfterTool, decision.finishReason)
	})

	t.Run("ErrorFinalizerResultRetries", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
			chatMessageWithParts(t, 2, database.ChatMessageRoleAssistant, []codersdk.ChatMessagePart{
				codersdk.ChatMessageToolCall("call_1", structuredoutput.ToolName, json.RawMessage(`{"output":{}}`)),
			}),
			chatMessageWithParts(t, 3, database.ChatMessageRoleTool, []codersdk.ChatMessagePart{
				codersdk.ChatMessageToolResult("call_1", structuredoutput.ToolName, json.RawMessage(`{"error":"missing answer"}`), true, false),
			}),
		}
		decision, err := decideGenerationAction(generationDecisionInput{
			messages:                 messages,
			stopAfterTools:           stopAfter,
			structuredOutputRequired: true,
			maxSteps:                 10,
		})
		require.NoError(t, err)
		require.Equal(t, generationActionGenerateAssistant, decision.kind)
	})

	t.Run("MaxStepsWithoutFinalizerFailsTerminally", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
			textAssistantMessage(t, 2, "text one"),
			textAssistantMessage(t, 3, "text two"),
		}
		_, err := decideGenerationAction(generationDecisionInput{
			messages:                 messages,
			stopAfterTools:           stopAfter,
			structuredOutputRequired: true,
			maxSteps:                 2,
		})
		require.Error(t, err)
		require.True(t, isTerminalGeneration(err))
		require.ErrorIs(t, err, errStructuredOutputNotProduced)
	})

	t.Run("MaxStepsWithoutStructuredOutputFinishes", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "plain", nil),
			chatMessageWithParts(t, 2, database.ChatMessageRoleAssistant, []codersdk.ChatMessagePart{
				codersdk.ChatMessageToolCall("call_1", "read_file", json.RawMessage(`{}`)),
			}),
			chatMessageWithParts(t, 3, database.ChatMessageRoleTool, []codersdk.ChatMessagePart{
				codersdk.ChatMessageToolResult("call_1", "read_file", json.RawMessage(`{}`), false, false),
			}),
		}
		decision, err := decideGenerationAction(generationDecisionInput{
			messages: messages,
			maxSteps: 1,
		})
		require.NoError(t, err)
		require.Equal(t, generationActionFinishTurn, decision.kind)
		require.Equal(t, generationFinishReasonMaxSteps, decision.finishReason)
	})
}
