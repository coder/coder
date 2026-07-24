package chatprompt_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
)

// userFileMessage builds a single persisted user message holding one
// file_id-backed file part, plus a resolver that returns the given
// bytes, media type, and filename for that file.
func userFileMessage(t *testing.T, name, mediaType string, data []byte) ([]database.ChatMessage, chatprompt.FileResolver) {
	t.Helper()
	fileID := uuid.New()
	rawContent := mustJSON(t, []json.RawMessage{
		mustJSON(t, map[string]any{
			"type": "file",
			"data": map[string]any{
				"media_type": mediaType,
				"file_id":    fileID.String(),
			},
		}),
	})
	messages := []database.ChatMessage{{
		Role:       database.ChatMessageRoleUser,
		Visibility: database.ChatMessageVisibilityBoth,
		Content:    pqtype.NullRawMessage{RawMessage: rawContent, Valid: true},
	}}
	resolver := func(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]chatprompt.FileData, error) {
		result := make(map[uuid.UUID]chatprompt.FileData)
		for _, id := range ids {
			if id == fileID {
				result[id] = chatprompt.FileData{
					Name:      name,
					Data:      data,
					MediaType: mediaType,
				}
			}
		}
		return result, nil
	}
	return messages, resolver
}

func acceptAll(string) bool  { return true }
func acceptNone(string) bool { return false }

func TestConvertMessagesWithFiles_InlinesTextFilePartWhenProviderRejects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		fileName  string
		mediaType string
		data      []byte
		accepts   func(string) bool
		wantText  bool // true = TextPart, false = FilePart
	}{
		{
			name:      "json rejected becomes text",
			fileName:  "data.json",
			mediaType: "application/json",
			data:      []byte(`{"hello":"world"}`),
			accepts:   acceptNone,
			wantText:  true,
		},
		{
			name:      "text accepted stays file",
			fileName:  "notes.txt",
			mediaType: "text/plain",
			data:      []byte("plain text body"),
			accepts:   acceptAll,
			wantText:  false,
		},
		{
			name:      "text rejected becomes text",
			fileName:  "notes.txt",
			mediaType: "text/plain",
			data:      []byte("plain text body"),
			accepts:   acceptNone,
			wantText:  true,
		},
		{
			name:      "markdown rejected becomes text",
			fileName:  "README.md",
			mediaType: "text/markdown",
			data:      []byte("# Title"),
			accepts:   acceptNone,
			wantText:  true,
		},
		{
			name:      "image never decoded even when rejected",
			fileName:  "pic.png",
			mediaType: "image/png",
			data:      []byte("not really png"),
			accepts:   acceptNone,
			wantText:  false,
		},
		{
			name:      "octet-stream never decoded even when rejected",
			fileName:  "blob.bin",
			mediaType: "application/octet-stream",
			data:      []byte("binary"),
			accepts:   acceptNone,
			wantText:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			messages, resolver := userFileMessage(t, tc.fileName, tc.mediaType, tc.data)
			prompt, err := chatprompt.ConvertMessagesWithFiles(
				context.Background(),
				messages,
				resolver,
				slogtest.Make(t, nil),
				tc.accepts,
			)
			require.NoError(t, err)
			require.Len(t, prompt, 1)
			require.Len(t, prompt[0].Content, 1)

			if tc.wantText {
				textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[0].Content[0])
				require.True(t, ok, "expected TextPart")
				require.Contains(t, textPart.Text, tc.fileName)
				require.Contains(t, textPart.Text, string(tc.data))
				return
			}

			filePart, ok := fantasy.AsMessagePart[fantasy.FilePart](prompt[0].Content[0])
			require.True(t, ok, "expected FilePart")
			require.Equal(t, tc.data, filePart.Data)
		})
	}
}

func TestConvertMessagesWithFiles_NilPredicateKeepsFilePart(t *testing.T) {
	t.Parallel()

	data := []byte(`{"a":1}`)
	messages, resolver := userFileMessage(t, "data.json", "application/json", data)
	prompt, err := chatprompt.ConvertMessagesWithFiles(
		context.Background(),
		messages,
		resolver,
		slogtest.Make(t, nil),
		nil,
	)
	require.NoError(t, err)
	require.Len(t, prompt, 1)
	require.Len(t, prompt[0].Content, 1)
	filePart, ok := fantasy.AsMessagePart[fantasy.FilePart](prompt[0].Content[0])
	require.True(t, ok, "expected FilePart when predicate is nil")
	require.Equal(t, data, filePart.Data)
}

func TestConvertMessagesWithFiles_InlinedTextNotTruncated(t *testing.T) {
	t.Parallel()

	// A large text file is inlined in full, with no silent truncation,
	// matching how a provider that accepts the media type natively would
	// receive the whole file.
	const budget = 128 * 1024
	data := []byte(strings.Repeat("a", budget+1024))
	messages, resolver := userFileMessage(t, "big.txt", "text/plain", data)
	prompt, err := chatprompt.ConvertMessagesWithFiles(
		context.Background(),
		messages,
		resolver,
		slogtest.Make(t, nil),
		acceptNone,
	)
	require.NoError(t, err)
	require.Len(t, prompt, 1)
	require.Len(t, prompt[0].Content, 1)
	textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[0].Content[0])
	require.True(t, ok, "expected TextPart")
	require.Contains(t, textPart.Text, string(data),
		"the full file content should be inlined without truncation")
	require.NotContains(t, textPart.Text, "truncated")
}
