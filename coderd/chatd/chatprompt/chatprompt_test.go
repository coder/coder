package chatprompt_test

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/database"
)

func TestConvertMessages_NormalizesAssistantToolCallInput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: "{}",
		},
		{
			name:     "invalid json",
			input:    "{\"command\":",
			expected: "{}",
		},
		{
			name:     "non-object json",
			input:    "[]",
			expected: "{}",
		},
		{
			name:     "valid object json",
			input:    "{\"command\":\"ls\"}",
			expected: "{\"command\":\"ls\"}",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assistantContent, err := chatprompt.MarshalContent([]fantasy.Content{
				fantasy.ToolCallContent{
					ToolCallID: "toolu_01C4PqN6F2493pi7Ebag8Vg7",
					ToolName:   "execute",
					Input:      tc.input,
				},
			}, nil)
			require.NoError(t, err)

			toolContent, err := chatprompt.MarshalToolResult(
				"toolu_01C4PqN6F2493pi7Ebag8Vg7",
				"execute",
				json.RawMessage(`{"error":"tool call was interrupted before it produced a result"}`),
				true,
			)
			require.NoError(t, err)

			prompt, err := chatprompt.ConvertMessages([]database.ChatMessage{
				{
					Role:       string(fantasy.MessageRoleAssistant),
					Visibility: database.ChatMessageVisibilityBoth,
					Content:    assistantContent,
				},
				{
					Role:       string(fantasy.MessageRoleTool),
					Visibility: database.ChatMessageVisibilityBoth,
					Content:    toolContent,
				},
			})
			require.NoError(t, err)
			require.Len(t, prompt, 2)

			require.Equal(t, fantasy.MessageRoleAssistant, prompt[0].Role)
			toolCalls := chatprompt.ExtractToolCalls(prompt[0].Content)
			require.Len(t, toolCalls, 1)
			require.Equal(t, tc.expected, toolCalls[0].Input)
			require.Equal(t, "execute", toolCalls[0].ToolName)
			require.Equal(t, "toolu_01C4PqN6F2493pi7Ebag8Vg7", toolCalls[0].ToolCallID)

			require.Equal(t, fantasy.MessageRoleTool, prompt[1].Role)
		})
	}
}

func TestConvertMessagesWithFiles_ResolvesFileData(t *testing.T) {
	t.Parallel()

	fileID := uuid.New()
	fileData := []byte("fake-image-bytes")

	// Build a user message with file_id but no inline data, as
	// would be stored after injectFileID strips the data.
	rawContent := mustJSON(t, []json.RawMessage{
		mustJSON(t, map[string]any{
			"type": "file",
			"data": map[string]any{
				"media_type": "image/png",
				"file_id":    fileID.String(),
			},
		}),
	})

	resolver := func(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]chatprompt.FileData, error) {
		result := make(map[uuid.UUID]chatprompt.FileData)
		for _, id := range ids {
			if id == fileID {
				result[id] = chatprompt.FileData{
					Data:      fileData,
					MediaType: "image/png",
				}
			}
		}
		return result, nil
	}

	prompt, err := chatprompt.ConvertMessagesWithFiles(
		context.Background(),
		[]database.ChatMessage{
			{
				Role:       string(fantasy.MessageRoleUser),
				Visibility: database.ChatMessageVisibilityBoth,
				Content:    pqtype.NullRawMessage{RawMessage: rawContent, Valid: true},
			},
		},
		resolver,
	)
	require.NoError(t, err)
	require.Len(t, prompt, 1)
	require.Equal(t, fantasy.MessageRoleUser, prompt[0].Role)
	require.Len(t, prompt[0].Content, 1)

	filePart, ok := fantasy.AsMessagePart[fantasy.FilePart](prompt[0].Content[0])
	require.True(t, ok, "expected FilePart")
	require.Equal(t, fileData, filePart.Data)
	require.Equal(t, "image/png", filePart.MediaType)
}

func TestConvertMessagesWithFiles_BackwardCompat(t *testing.T) {
	t.Parallel()

	// A message with inline data and a file_id should use the
	// inline data even when the resolver returns nothing.
	fileID := uuid.New()
	inlineData := []byte("inline-image-data")

	rawContent := mustJSON(t, []json.RawMessage{
		mustJSON(t, map[string]any{
			"type": "file",
			"data": map[string]any{
				"media_type": "image/png",
				"data":       inlineData,
				"file_id":    fileID.String(),
			},
		}),
	})

	prompt, err := chatprompt.ConvertMessagesWithFiles(
		context.Background(),
		[]database.ChatMessage{
			{
				Role:       string(fantasy.MessageRoleUser),
				Visibility: database.ChatMessageVisibilityBoth,
				Content:    pqtype.NullRawMessage{RawMessage: rawContent, Valid: true},
			},
		},
		nil, // No resolver.
	)
	require.NoError(t, err)
	require.Len(t, prompt, 1)
	require.Len(t, prompt[0].Content, 1)

	filePart, ok := fantasy.AsMessagePart[fantasy.FilePart](prompt[0].Content[0])
	require.True(t, ok, "expected FilePart")
	require.Equal(t, inlineData, filePart.Data)
}

func TestInjectFileID_StripsInlineData(t *testing.T) {
	t.Parallel()

	fileID := uuid.New()
	imageData := []byte("raw-image-bytes")

	// Marshal a file content block with inline data, then inject
	// a file_id. The result should have file_id but no data.
	content, err := chatprompt.MarshalContent([]fantasy.Content{
		fantasy.FileContent{
			MediaType: "image/png",
			Data:      imageData,
		},
	}, map[int]uuid.UUID{0: fileID})
	require.NoError(t, err)

	// Parse the stored content to verify shape.
	var blocks []json.RawMessage
	require.NoError(t, json.Unmarshal(content.RawMessage, &blocks))
	require.Len(t, blocks, 1)

	var envelope struct {
		Type string `json:"type"`
		Data struct {
			MediaType string           `json:"media_type"`
			Data      *json.RawMessage `json:"data,omitempty"`
			FileID    string           `json:"file_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(blocks[0], &envelope))
	require.Equal(t, "file", envelope.Type)
	require.Equal(t, "image/png", envelope.Data.MediaType)
	require.Equal(t, fileID.String(), envelope.Data.FileID)
	// Data should be nil (omitted) since injectFileID strips it.
	require.Nil(t, envelope.Data.Data, "inline data should be stripped")
}

// TestConvertMessages_TrailingAssistantTextOnly verifies that when an
// interrupted agent persists a text-only assistant message after the
// user's interrupt message (a race inherent to the interrupt flow),
// the resulting prompt does not end with an assistant message. The
// Anthropic API interprets a trailing assistant message as "prefill"
// and rejects it on models that do not support that feature.
func TestConvertMessages_TrailingAssistantTextOnly(t *testing.T) {
	t.Parallel()

	// Simulate the DB order after interrupt: the user message was
	// inserted at T1, then the partial assistant text was persisted
	// at T2 (T2 > T1), so the assistant message is last.
	assistantContent, err := chatprompt.MarshalContent([]fantasy.Content{
		fantasy.TextContent{Text: "partial response before interrupt"},
	}, nil)
	require.NoError(t, err)

	userContent, err := json.Marshal([]json.RawMessage{
		mustJSON(t, map[string]any{
			"type": "text",
			"data": map[string]any{"text": "initial request"},
		}),
	})
	require.NoError(t, err)

	interruptContent, err := json.Marshal([]json.RawMessage{
		mustJSON(t, map[string]any{
			"type": "text",
			"data": map[string]any{"text": "new instruction after interrupt"},
		}),
	})
	require.NoError(t, err)

	prompt, err := chatprompt.ConvertMessages([]database.ChatMessage{
		{
			Role:       string(fantasy.MessageRoleUser),
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
		},
		// The interrupt user message was inserted before the partial
		// assistant response was persisted.
		{
			Role:       string(fantasy.MessageRoleUser),
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    pqtype.NullRawMessage{RawMessage: interruptContent, Valid: true},
		},
		// Partial assistant text persisted after the interrupt.
		{
			Role:       string(fantasy.MessageRoleAssistant),
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    assistantContent,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, prompt)

	lastMsg := prompt[len(prompt)-1]
	require.NotEqual(t, fantasy.MessageRoleAssistant, lastMsg.Role,
		"prompt must not end with an assistant message; "+
			"Anthropic rejects this as unsupported prefill")

	// The reordered prompt should place the assistant message
	// before the interrupt user message.
	require.Len(t, prompt, 3)
	require.Equal(t, fantasy.MessageRoleUser, prompt[0].Role)
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[1].Role)
	require.Equal(t, fantasy.MessageRoleUser, prompt[2].Role)
}

// TestConvertMessages_TrailingAssistantWithToolCalls verifies that
// a trailing assistant message containing tool calls is NOT
// reordered. The existing injectMissingToolResults logic already
// appends synthetic tool-result messages (role=tool) after such
// assistant messages, so the prompt naturally ends with a
// tool/user turn.
func TestConvertMessages_TrailingAssistantWithToolCalls(t *testing.T) {
	t.Parallel()

	assistantContent, err := chatprompt.MarshalContent([]fantasy.Content{
		fantasy.TextContent{Text: "let me check that"},
		fantasy.ToolCallContent{
			ToolCallID: "toolu_interrupted",
			ToolName:   "read_file",
			Input:      `{"path":"main.go"}`,
		},
	}, nil)
	require.NoError(t, err)

	userContent, err := json.Marshal([]json.RawMessage{
		mustJSON(t, map[string]any{
			"type": "text",
			"data": map[string]any{"text": "do something"},
		}),
	})
	require.NoError(t, err)

	prompt, err := chatprompt.ConvertMessages([]database.ChatMessage{
		{
			Role:       string(fantasy.MessageRoleUser),
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
		},
		{
			Role:       string(fantasy.MessageRoleAssistant),
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    assistantContent,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, prompt)

	// injectMissingToolResults should have added a synthetic tool
	// result, so the last message should be a tool message.
	lastMsg := prompt[len(prompt)-1]
	require.Equal(t, fantasy.MessageRoleTool, lastMsg.Role,
		"trailing assistant with tool calls should get synthetic "+
			"tool results appended by injectMissingToolResults")
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}
