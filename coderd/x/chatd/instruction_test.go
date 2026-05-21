package chatd //nolint:testpackage // Uses internal symbols.

import (
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

func TestRenderPlanPathPrompt(t *testing.T) {
	t.Parallel()

	newPromptWithPlaceholder := func() []fantasy.Message {
		return []fantasy.Message{
			{
				Role: fantasy.MessageRoleSystem,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "<planning>\n" + defaultSystemPromptPlanPathBlockPlaceholder + "\n</planning>"},
				},
			},
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "hello"},
				},
			},
		}
	}

	messageText := func(t *testing.T, message fantasy.Message) string {
		t.Helper()
		part, ok := fantasy.AsMessagePart[fantasy.TextPart](message.Content[0])
		require.True(t, ok)
		return part.Text
	}

	t.Run("ReplacesPlaceholderWithResolvedHome", func(t *testing.T) {
		t.Parallel()

		prompt := newPromptWithPlaceholder()
		got := renderPlanPathPrompt(prompt, formatPlanPathBlock(
			"/Users/dev/.coder/plans/PLAN-chat.md",
			"/Users/dev",
		))

		require.Len(t, got, len(prompt))
		text := messageText(t, got[0])
		require.Contains(t, text, "Your plan file path for this chat is: /Users/dev/.coder/plans/PLAN-chat.md")
		require.Contains(t, text, "Do not use /Users/dev/PLAN.md.")
		require.NotContains(t, text, defaultSystemPromptPlanPathBlockPlaceholder)
	})

	t.Run("FallsBackToLegacySharedPathWhenHomeIsEmpty", func(t *testing.T) {
		t.Parallel()

		prompt := newPromptWithPlaceholder()
		got := renderPlanPathPrompt(prompt, formatPlanPathBlock(
			"/home/coder/.coder/plans/PLAN-chat.md",
			"",
		))

		text := messageText(t, got[0])
		require.Contains(t, text, "Do not use "+chattool.LegacySharedPlanPath+".")
	})

	t.Run("LeavesPromptUnchangedWhenPlaceholderMissing", func(t *testing.T) {
		t.Parallel()

		prompt := []fantasy.Message{
			{
				Role: fantasy.MessageRoleSystem,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "base instructions"},
				},
			},
			{
				Role: fantasy.MessageRoleSystem,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "workspace awareness"},
				},
			},
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "hello"},
				},
			},
		}

		got := renderPlanPathPrompt(prompt, formatPlanPathBlock(
			"/home/coder/.coder/plans/PLAN-chat.md",
			"/home/coder",
		))

		require.Equal(t, prompt, got)
	})

	t.Run("RemovesPlaceholderWhenPlanPathBlockIsEmpty", func(t *testing.T) {
		t.Parallel()

		prompt := newPromptWithPlaceholder()
		got := renderPlanPathPrompt(prompt, "")

		require.Len(t, got, len(prompt))
		text := messageText(t, got[0])
		require.NotContains(t, text, defaultSystemPromptPlanPathBlockPlaceholder)
		require.NotContains(t, text, "<plan-file-path>")
	})
}

func TestInsertSystemInstructionAfterSystemMessages(t *testing.T) {
	t.Parallel()

	prompt := []fantasy.Message{
		{
			Role: fantasy.MessageRoleSystem,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "base"},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "hello"},
			},
		},
	}

	got := chatprompt.InsertSystem(prompt, "project rules")
	require.Len(t, got, 3)
	require.Equal(t, fantasy.MessageRoleSystem, got[0].Role)
	require.Equal(t, fantasy.MessageRoleSystem, got[1].Role)
	require.Equal(t, fantasy.MessageRoleUser, got[2].Role)

	part, ok := fantasy.AsMessagePart[fantasy.TextPart](got[1].Content[0])
	require.True(t, ok)
	require.Equal(t, "project rules", part.Text)
}

func TestFormatSystemInstructions(t *testing.T) {
	t.Parallel()

	t.Run("HomeAndPwdWithAgentContext", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("linux", "/home/coder/project", []codersdk.ChatMessagePart{
			{Type: codersdk.ChatMessagePartTypeContextFile, ContextFileContent: "home rules", ContextFilePath: "/home/coder/.coder/AGENTS.md"},
			{Type: codersdk.ChatMessagePartTypeContextFile, ContextFileContent: "project rules", ContextFilePath: "/home/coder/project/AGENTS.md"},
		})
		require.Contains(t, got, "Operating System: linux")
		require.Contains(t, got, "Working Directory: /home/coder/project")
		require.Contains(t, got, "Source: /home/coder/.coder/AGENTS.md")
		require.Contains(t, got, "home rules")
		require.Contains(t, got, "Source: /home/coder/project/AGENTS.md")
		require.Contains(t, got, "project rules")
		require.True(t, strings.HasPrefix(got, "<workspace-context>"))
		require.True(t, strings.HasSuffix(got, "</workspace-context>"))
	})

	t.Run("OnlyPwdFile", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("", "/home/coder/project", []codersdk.ChatMessagePart{
			{Type: codersdk.ChatMessagePartTypeContextFile, ContextFileContent: "project rules", ContextFilePath: "/home/coder/project/AGENTS.md"},
		})
		require.Contains(t, got, "project rules")
		require.Contains(t, got, "Source: /home/coder/project/AGENTS.md")
		require.NotContains(t, got, ".coder/AGENTS.md")
	})

	t.Run("OnlyAgentContext", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("darwin", "/Users/dev/repo", nil)
		require.Contains(t, got, "Operating System: darwin")
		require.Contains(t, got, "Working Directory: /Users/dev/repo")
		require.NotContains(t, got, "Source:")
		require.True(t, strings.HasPrefix(got, "<workspace-context>"))
		require.True(t, strings.HasSuffix(got, "</workspace-context>"))
	})

	t.Run("OnlyHomeFile", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("", "", []codersdk.ChatMessagePart{
			{Type: codersdk.ChatMessagePartTypeContextFile, ContextFileContent: "home rules", ContextFilePath: "~/.coder/AGENTS.md"},
		})
		require.Contains(t, got, "Source: ~/.coder/AGENTS.md")
		require.Contains(t, got, "home rules")
		require.NotContains(t, got, "Operating System:")
		require.NotContains(t, got, "Working Directory:")
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("", "", nil)
		require.Empty(t, got)
	})

	t.Run("TruncatedFile", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("windows", "", []codersdk.ChatMessagePart{
			{Type: codersdk.ChatMessagePartTypeContextFile, ContextFileContent: "rules", ContextFilePath: "/path/AGENTS.md", ContextFileTruncated: true},
		})
		require.Contains(t, got, "truncated to 64KiB")
		require.Contains(t, got, "Operating System: windows")
	})

	t.Run("AgentContextBeforeFiles", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("linux", "/home/project", []codersdk.ChatMessagePart{
			{Type: codersdk.ChatMessagePartTypeContextFile, ContextFileContent: "home", ContextFilePath: "/home/.coder/AGENTS.md"},
			{Type: codersdk.ChatMessagePartTypeContextFile, ContextFileContent: "pwd", ContextFilePath: "/home/project/AGENTS.md"},
		})
		osIdx := strings.Index(got, "Operating System:")
		dirIdx := strings.Index(got, "Working Directory:")
		homeSourceIdx := strings.Index(got, "Source: /home/.coder/AGENTS.md")
		pwdSourceIdx := strings.Index(got, "Source: /home/project/AGENTS.md")
		require.Less(t, osIdx, homeSourceIdx)
		require.Less(t, dirIdx, homeSourceIdx)
		require.Less(t, homeSourceIdx, pwdSourceIdx)
	})

	t.Run("EmptySectionsIgnored", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("linux", "", []codersdk.ChatMessagePart{
			{Type: codersdk.ChatMessagePartTypeContextFile, ContextFileContent: "", ContextFilePath: "/empty"},
			{Type: codersdk.ChatMessagePartTypeContextFile, ContextFileContent: "real", ContextFilePath: "/real/AGENTS.md"},
		})
		require.NotContains(t, got, "Source: /empty")
		require.Contains(t, got, "Source: /real/AGENTS.md")
	})
}

func TestInstructionFromContextFiles(t *testing.T) {
	t.Parallel()

	makeMsg := func(parts []codersdk.ChatMessagePart) database.ChatMessage {
		raw, _ := json.Marshal(parts)
		return database.ChatMessage{
			Content: pqtype.NullRawMessage{RawMessage: raw, Valid: true},
		}
	}

	t.Run("EmptyMessages", func(t *testing.T) {
		t.Parallel()
		got := instructionFromContextFiles(nil)
		require.Empty(t, got)
	})

	t.Run("NoContextFileParts", func(t *testing.T) {
		t.Parallel()
		msgs := []database.ChatMessage{
			makeMsg([]codersdk.ChatMessagePart{
				{
					Type:             codersdk.ChatMessagePartTypeSkill,
					SkillName:        "test",
					SkillDescription: "test skill",
				},
			}),
		}
		got := instructionFromContextFiles(msgs)
		require.Empty(t, got)
	})

	t.Run("ReconstructsFromContextFileParts", func(t *testing.T) {
		t.Parallel()
		msgs := []database.ChatMessage{
			makeMsg([]codersdk.ChatMessagePart{
				{
					Type:                 codersdk.ChatMessagePartTypeContextFile,
					ContextFileOS:        "linux",
					ContextFileDirectory: "/home/coder/project",
					ContextFileContent:   "project rules",
					ContextFilePath:      "/home/coder/project/AGENTS.md",
				},
			}),
		}
		got := instructionFromContextFiles(msgs)
		require.Contains(t, got, "Operating System: linux")
		require.Contains(t, got, "Working Directory: /home/coder/project")
		require.Contains(t, got, "Source: /home/coder/project/AGENTS.md")
		require.Contains(t, got, "project rules")
	})
}
