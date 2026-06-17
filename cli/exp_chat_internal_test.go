package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestRenderChatContextText(t *testing.T) {
	t.Parallel()

	chatID := uuid.MustParse("11111111-1111-4111-8111-111111111111")

	t.Run("NoPinnedContext", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		require.NoError(t, renderChatContextText(&buf, codersdk.Chat{ID: chatID}))
		require.Contains(t, buf.String(), "has no pinned workspace context")
	})

	t.Run("CleanListsResourcesWithoutChanges", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		require.NoError(t, renderChatContextText(&buf, codersdk.Chat{
			ID: chatID,
			Context: &codersdk.ChatContext{
				Dirty: false,
				Resources: []codersdk.ChatContextResource{
					{
						Source:    "/home/coder/AGENTS.md",
						Kind:      codersdk.ChatContextResourceKindInstructionFile,
						SizeBytes: 12,
					},
					{
						Source:    "/home/coder/.coder/skills/deploy",
						Kind:      codersdk.ChatContextResourceKindSkill,
						SizeBytes: 34,
						SkillName: "deploy",
					},
				},
			},
		}))
		out := buf.String()
		require.Contains(t, out, "Status: clean")
		require.Contains(t, out, "/home/coder/AGENTS.md")
		require.Contains(t, out, "deploy")
		// A clean chat shows no change section or refresh hint.
		require.NotContains(t, out, "Changes vs latest snapshot")
		require.NotContains(t, out, "refresh")
	})

	t.Run("DirtyShowsChangesAndRefreshHint", func(t *testing.T) {
		t.Parallel()

		since := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
		var buf bytes.Buffer
		require.NoError(t, renderChatContextText(&buf, codersdk.Chat{
			ID: chatID,
			Context: &codersdk.ChatContext{
				Dirty:      true,
				DirtySince: &since,
				Error:      "two sources failed to resolve",
				Resources: []codersdk.ChatContextResource{
					{
						Source: "/home/coder/AGENTS.md",
						Kind:   codersdk.ChatContextResourceKindInstructionFile,
					},
				},
				Changes: []codersdk.ChatContextResourceChange{
					{
						Source:     "/home/coder/AGENTS.md",
						Kind:       codersdk.ChatContextResourceKindInstructionFile,
						Status:     codersdk.ChatContextResourceChangeStatusModified,
						OldContent: "old",
						NewContent: "new",
					},
					{
						Source:    "/home/coder/.coder/skills/deploy",
						Kind:      codersdk.ChatContextResourceKindSkill,
						Status:    codersdk.ChatContextResourceChangeStatusAdded,
						SkillName: "deploy",
					},
				},
			},
		}))
		out := buf.String()
		require.Contains(t, out, "Status: drifted (since 2024-01-02T03:04:05Z)")
		require.Contains(t, out, "two sources failed to resolve")
		require.Contains(t, out, "Changes vs latest snapshot (2)")
		require.Contains(t, out, "modified")
		require.Contains(t, out, "added")
		require.Contains(t, out, "chat context refresh 11111111-1111-4111-8111-111111111111")
	})
}
