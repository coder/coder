package chatprompt_test

import (
	"context"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

var testChatWorkspaceID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

func TestWorkspaceFilePartToText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		part    codersdk.ChatMessagePart
		wantSub []string
	}{
		{
			name: "with_size",
			part: codersdk.ChatMessagePart{
				Type:                     codersdk.ChatMessagePartTypeWorkspaceFileReference,
				WorkspaceFileWorkspaceID: testChatWorkspaceID,
				WorkspaceFileName:        "archive.zip",
				WorkspaceFilePath:        "/home/coder/.coder/chats/abcd1234/files/archive.zip",
				WorkspaceFileSize:        2048,
			},
			wantSub: []string{"archive.zip", "/home/coder/.coder/chats/abcd1234/files/archive.zip", "2.0 KiB"},
		},
		{
			name: "no_size_omits_paren",
			part: codersdk.ChatMessagePart{
				Type:                     codersdk.ChatMessagePartTypeWorkspaceFileReference,
				WorkspaceFileWorkspaceID: testChatWorkspaceID,
				WorkspaceFileName:        "foo.txt",
				WorkspaceFilePath:        "/home/coder/foo.txt",
				WorkspaceFileSize:        0,
			},
			wantSub: []string{"foo.txt", "/home/coder/foo.txt"},
		},
		{
			name: "mib_unit",
			part: codersdk.ChatMessagePart{
				Type:                     codersdk.ChatMessagePartTypeWorkspaceFileReference,
				WorkspaceFileWorkspaceID: testChatWorkspaceID,
				WorkspaceFileName:        "big.bin",
				WorkspaceFilePath:        "/home/coder/big.bin",
				WorkspaceFileSize:        5 * 1024 * 1024,
			},
			wantSub: []string{"big.bin", "5.0 MiB"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := chatprompt.WorkspaceFilePartToTextForTest(tt.part, uuid.NullUUID{UUID: testChatWorkspaceID, Valid: true})
			require.True(t, strings.HasPrefix(got, "[workspace file: "), "expected prefix, got %q", got)
			require.True(t, strings.HasSuffix(got, "]"), "expected closing bracket, got %q", got)
			for _, sub := range tt.wantSub {
				require.Contains(t, got, sub)
			}
			if tt.part.WorkspaceFileSize == 0 {
				require.NotContains(t, got, "(")
			}
		})
	}
}

func TestWorkspaceFilePartToTextStaleWorkspace(t *testing.T) {
	t.Parallel()

	part := codersdk.ChatMessagePart{
		Type:                     codersdk.ChatMessagePartTypeWorkspaceFileReference,
		WorkspaceFileWorkspaceID: testChatWorkspaceID,
		WorkspaceFileName:        "archive.zip",
		WorkspaceFilePath:        "/home/coder/.coder/chats/abcd1234/files/archive.zip",
		WorkspaceFileSize:        2048,
	}

	t.Run("ReboundWorkspace", func(t *testing.T) {
		t.Parallel()
		got := chatprompt.WorkspaceFilePartToTextForTest(part, uuid.NullUUID{UUID: uuid.MustParse("22222222-2222-2222-2222-222222222222"), Valid: true})
		require.Contains(t, got, "archive.zip")
		require.Contains(t, got, "2.0 KiB")
		require.Contains(t, got, "no longer accessible")
		require.NotContains(t, got, "/home/coder/.coder/chats", "stale path must not steer the model")
	})

	t.Run("UnboundChat", func(t *testing.T) {
		t.Parallel()
		got := chatprompt.WorkspaceFilePartToTextForTest(part, uuid.NullUUID{})
		require.Contains(t, got, "no longer accessible")
		require.NotContains(t, got, "/home/coder/.coder/chats")
	})

	t.Run("MatchingWorkspaceKeepsPath", func(t *testing.T) {
		t.Parallel()
		got := chatprompt.WorkspaceFilePartToTextForTest(part, uuid.NullUUID{UUID: testChatWorkspaceID, Valid: true})
		require.Contains(t, got, "/home/coder/.coder/chats/abcd1234/files/archive.zip")
		require.NotContains(t, got, "no longer accessible")
	})
}

func TestWorkspaceAttachmentPromptText(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	attachmentID := uuid.New()
	promptFileID := uuid.New()
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("unpack this"),
		codersdk.ChatMessageWorkspaceAttachment(attachmentID, "application/zip", "report.zip", 62<<20),
		codersdk.ChatMessageFile(promptFileID, "image/png", "screen.png"),
	})
	require.NoError(t, err)
	messages := []database.ChatMessage{{
		ChatID:         chatID,
		Role:           database.ChatMessageRoleUser,
		Visibility:     database.ChatMessageVisibilityBoth,
		Content:        content,
		ContentVersion: chatprompt.CurrentContentVersion,
	}}

	var resolvedIDs [][]uuid.UUID
	resolver := func(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]chatprompt.FileData, error) {
		resolvedIDs = append(resolvedIDs, ids)
		return map[uuid.UUID]chatprompt.FileData{
			promptFileID: {Name: "screen.png", Data: []byte("png"), MediaType: "image/png"},
		}, nil
	}
	var storedAttachments map[uuid.UUID]chatprompt.FileData
	var attachmentIDs [][]uuid.UUID
	attachmentResolver := func(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]chatprompt.FileData, error) {
		attachmentIDs = append(attachmentIDs, ids)
		return storedAttachments, nil
	}
	attachmentText := func(chatWorkspaceID uuid.NullUUID) string {
		t.Helper()
		prompt, err := chatprompt.ConvertMessagesWithFiles(
			context.Background(), messages, resolver, attachmentResolver, slogtest.Make(t, nil), nil, chatWorkspaceID,
		)
		require.NoError(t, err)
		require.Len(t, prompt, 1)
		require.Len(t, prompt[0].Content, 3)
		text, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[0].Content[1])
		require.True(t, ok)
		return text.Text
	}
	homePath := "~/.coder/chats/" + chatID.String() + "/files/" + attachmentID.String() + "/report.zip"

	storedAttachments = map[uuid.UUID]chatprompt.FileData{attachmentID: {Name: "report.zip", MediaType: "application/zip"}}
	unbound := attachmentText(uuid.NullUUID{})
	bound := attachmentText(uuid.NullUUID{UUID: testChatWorkspaceID, Valid: true})
	require.Equal(t, unbound, bound, "the text must not depend on the chat's workspace")
	require.Contains(t, unbound, "report.zip (application/zip, 62 MiB)")
	require.Contains(t, unbound, "Coder copies it to "+homePath)
	require.Contains(t, unbound, "copy_attachments_to_workspace")
	require.Equal(t, [][]uuid.UUID{{promptFileID}, {promptFileID}}, resolvedIDs,
		"workspace attachment bytes must never be loaded into the prompt")
	require.Equal(t, [][]uuid.UUID{{attachmentID}, {attachmentID}}, attachmentIDs)

	// The attachment cap evicted the chat file. It may have been copied
	// before, so the text keeps the path but no longer promises a copy.
	storedAttachments = nil
	evicted := attachmentText(uuid.NullUUID{})
	require.Contains(t, evicted, "report.zip (application/zip, 62 MiB)")
	require.Contains(t, evicted, "Coder no longer stores it and will not copy it")
	require.Contains(t, evicted, "it may still be at "+homePath)
	require.NotContains(t, evicted, "Coder copies it to")
}
