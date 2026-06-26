package chatd

import (
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

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

func TestDefaultSystemPromptContainsVersionControlSafety(t *testing.T) {
	t.Parallel()

	require.Contains(t, DefaultSystemPrompt, "<version-control-safety>")
	require.Contains(t, DefaultSystemPrompt, "</version-control-safety>")
	require.Contains(t, DefaultSystemPrompt, "check the current branch and push target")
	require.Contains(t, DefaultSystemPrompt, "Do not commit directly to default or protected branches")
	require.Contains(t, DefaultSystemPrompt, "including main, master, trunk")
	require.Contains(t, DefaultSystemPrompt, "unless the user explicitly confirms after you identify the exact branch")
	require.Contains(t, DefaultSystemPrompt, "Do not push when the target would update a default or protected branch unless the user explicitly confirms")
	require.Contains(t, DefaultSystemPrompt, "Before asking for confirmation, warn that the push bypasses")
	require.Contains(t, DefaultSystemPrompt, "state the exact remote ref that would be updated")
	require.Contains(t, DefaultSystemPrompt, "Confirmation must be separate and must name the exact protected branch")
	require.Contains(t, DefaultSystemPrompt, "Do not run plain git push while checked out on a default or protected branch")
	require.Contains(t, DefaultSystemPrompt, "use an explicit refspec")
	require.Contains(t, DefaultSystemPrompt, "create and switch to a feature branch first")
	require.Contains(t, DefaultSystemPrompt, "Never treat the original request as confirmation")
}

func TestDefaultSystemPromptContainsSubagentOrchestration(t *testing.T) {
	t.Parallel()

	require.Contains(t, DefaultSystemPrompt, "<subagent-orchestration>")
	require.Contains(t, DefaultSystemPrompt, "</subagent-orchestration>")
	require.Contains(t, DefaultSystemPrompt, "An error status is often recoverable")
	require.Contains(t, DefaultSystemPrompt, "call list_agents to recover them")
}

func TestWorkspaceAwarenessDelaysWorkspaceCreation(t *testing.T) {
	t.Parallel()

	detached := workspaceDetachedAwareness
	require.Contains(t, detached, "No workspace is attached to this chat yet")
	require.Contains(t, detached, "Do not create or start a workspace by default")
	require.Contains(t, detached, "Only call create_workspace or start_workspace")
	require.NotContains(t, detached, "Create one using the create_workspace tool before using workspace tools")

	delegated := workspaceDetachedNoCreateAwareness
	require.Contains(t, delegated, "This delegated chat cannot create or start a workspace")
	require.Contains(t, delegated, "report that need to the parent agent")
	require.NotContains(t, delegated, "Only call create_workspace or start_workspace")

	attached := workspaceAttachedAwareness
	require.Contains(t, attached, "This chat is attached to a workspace")
}

func TestDefaultSystemPromptDelaysWorkspaceCreation(t *testing.T) {
	t.Parallel()

	require.Contains(t, DefaultSystemPrompt, "Do not create a workspace by default")
	require.Contains(t, DefaultSystemPrompt, "Do not clone repositories already present")
	require.Contains(t, DefaultSystemPrompt, "including AGENTS.md")
	require.NotContains(t, DefaultSystemPrompt, "create and start one first using create_workspace and start_workspace")
}

func TestPlanningOverlayPromptDelaysWorkspaceCreation(t *testing.T) {
	t.Parallel()

	prompt := PlanningOverlayPrompt()
	require.Contains(t, prompt, "do not create one as the first action merely because you are planning")
	require.Contains(t, prompt, "Before cloning, inspect the current workspace and reuse existing repositories")
	require.NotContains(t, prompt, "create and start one with create_workspace and start_workspace before investigating")
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
