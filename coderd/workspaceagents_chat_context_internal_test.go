package coderd

import (
	"encoding/json"
	"testing"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func TestCollectAgentChatContextPartsSkipsSentinelContextFiles(t *testing.T) {
	t.Parallel()

	content, err := json.Marshal([]codersdk.ChatMessagePart{
		{
			Type:            codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath: "/home/coder/project/.agents/skills/my-skill/SKILL.md",
		},
		{
			Type:             codersdk.ChatMessagePartTypeSkill,
			SkillName:        "my-skill",
			SkillDescription: "A test skill",
		},
		{
			Type:               codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath:    "/home/coder/project/AGENTS.md",
			ContextFileContent: "# Project instructions",
		},
		codersdk.ChatMessageText("ignored"),
	})
	require.NoError(t, err)

	parts, err := collectAgentChatContextParts([]database.ChatMessage{ //nolint:exhaustruct // Only content fields matter for this unit test.
		{
			ID: 1,
			Content: pqtype.NullRawMessage{
				RawMessage: content,
				Valid:      true,
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, parts, 2)
	require.Equal(t, codersdk.ChatMessagePartTypeSkill, parts[0].Type)
	require.Equal(t, "my-skill", parts[0].SkillName)
	require.Equal(t, codersdk.ChatMessagePartTypeContextFile, parts[1].Type)
	require.Equal(t, "/home/coder/project/AGENTS.md", parts[1].ContextFilePath)
	require.Equal(t, "# Project instructions", parts[1].ContextFileContent)
}
