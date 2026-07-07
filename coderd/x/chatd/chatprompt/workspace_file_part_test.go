package chatprompt_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

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
