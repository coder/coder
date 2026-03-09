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

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}
