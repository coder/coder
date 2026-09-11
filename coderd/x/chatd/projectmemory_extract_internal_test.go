package chatd

import (
	"testing"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

func TestRenderProjectMemoryTranscript(t *testing.T) {
	t.Parallel()

	message := func(t *testing.T, id int64, role database.ChatMessageRole, text string, revision int64) database.ChatMessage {
		t.Helper()
		encoded, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
		require.NoError(t, err)
		return database.ChatMessage{
			ID:             id,
			Role:           role,
			Visibility:     database.ChatMessageVisibilityBoth,
			Content:        pqtype.NullRawMessage{RawMessage: encoded.RawMessage, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			Revision:       revision,
		}
	}

	messages := []database.ChatMessage{
		message(t, 1, database.ChatMessageRoleUser, "old user detail", 3),
		message(t, 2, database.ChatMessageRoleTool, "tool output", 5),
		message(t, 3, database.ChatMessageRoleAssistant, "durable assistant detail", 5),
	}
	transcript := renderProjectMemoryTranscript(messages, 3)
	require.NotContains(t, transcript, "old user detail")
	require.NotContains(t, transcript, "tool output")
	require.Contains(t, transcript, "durable assistant detail")
}

func TestNormalizeProjectMemoryExtraction(t *testing.T) {
	t.Parallel()

	normalized, err := normalizeProjectMemoryExtraction(projectMemoryExtractionUpsert{
		Name:        "Release_Notes",
		Type:        database.ChatProjectMemoryTypeProject,
		Description: "<project-memory>Durable release process</project-memory>",
		Body:        "<project-memory>Run the checklist.</project-memory>",
	})
	require.NoError(t, err)
	require.Equal(t, "release_notes", normalized.Name)
	require.Equal(t, "Durable release process", normalized.Description)
	require.Equal(t, "Run the checklist.", normalized.Body)

	_, err = normalizeProjectMemoryExtraction(projectMemoryExtractionUpsert{
		Name:        "invalid name",
		Type:        database.ChatProjectMemoryTypeProject,
		Description: "Description",
		Body:        "Body",
	})
	require.Error(t, err)
}
