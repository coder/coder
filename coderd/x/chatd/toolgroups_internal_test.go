package chatd

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatadvisor"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

func TestResolveToolGroups(t *testing.T) {
	t.Parallel()

	expOn := func(codersdk.Experiment) bool { return true }
	expOff := func(codersdk.Experiment) bool { return false }

	t.Run("Defaults", func(t *testing.T) {
		t.Parallel()
		groups := resolveToolGroups(nil, expOff)
		for _, info := range codersdk.ChatToolGroupCatalog {
			require.Equal(t, info.DefaultOn, groups[info.ID], info.ID)
		}
	})

	t.Run("StoredOff", func(t *testing.T) {
		t.Parallel()
		groups := resolveToolGroups(map[codersdk.ChatToolGroup]bool{
			codersdk.ChatToolGroupSkills: false,
			codersdk.ChatToolGroupMemory: false,
		}, expOn)
		require.False(t, groups[codersdk.ChatToolGroupSkills])
		require.False(t, groups[codersdk.ChatToolGroupMemory])
		require.True(t, groups[codersdk.ChatToolGroupWorkspace])
		require.True(t, groups[codersdk.ChatToolGroupSubagents])
	})

	t.Run("FixedGroupIgnoresStored", func(t *testing.T) {
		t.Parallel()
		groups := resolveToolGroups(map[codersdk.ChatToolGroup]bool{
			codersdk.ChatToolGroupAdvisor: false,
		}, expOn)
		require.True(t, groups[codersdk.ChatToolGroupAdvisor])
	})

	t.Run("ExperimentOffKeepsStoredOff", func(t *testing.T) {
		t.Parallel()
		calls := 0
		groups := resolveToolGroups(map[codersdk.ChatToolGroup]bool{
			codersdk.ChatToolGroupSubagents: false,
			// Every switchable group is default on, so a stored on is
			// the default and needs no experiment read.
			codersdk.ChatToolGroupSkills: true,
		}, func(codersdk.Experiment) bool {
			calls++
			return false
		})
		require.False(t, groups[codersdk.ChatToolGroupSubagents])
		require.True(t, groups[codersdk.ChatToolGroupSkills])
		require.Equal(t, 1, calls, "experiment decided once per resolve")
	})

	t.Run("PlanningOffWhenWorkspaceOff", func(t *testing.T) {
		t.Parallel()
		groups := resolveToolGroups(map[codersdk.ChatToolGroup]bool{
			codersdk.ChatToolGroupWorkspace: false,
		}, expOn)
		require.False(t, groups[codersdk.ChatToolGroupWorkspace])
		require.False(t, groups[codersdk.ChatToolGroupPlanning])
		require.Equal(t, map[codersdk.ChatToolGroup]bool{
			codersdk.ChatToolGroupWorkspace: false,
		}, storedFormToolGroups(groups))
		reserved := reservedToolNames(groups)
		require.True(t, reserved["execute"])
		require.True(t, reserved["propose_plan"])
		require.False(t, reserved["spawn_agent"])
	})
}

// TestBuiltinToolGroupsCoverTurnTools fails when turn preparation can add a
// tool whose name has no group.
func TestBuiltinToolGroupsCoverTurnTools(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	server := &Server{}

	var tools []fantasy.AgentTool
	tools = append(tools, baseWorkspaceTools(baseWorkspaceToolsOptions{})...)
	tools = append(tools, chattool.NewAskUserQuestionTool())
	tools = server.appendRootChatTools(ctx, tools, rootChatToolsOptions{
		workspaceCtx:     &turnWorkspaceContext{},
		isPlanModeTurn:   true,
		storedToolGroups: map[codersdk.ChatToolGroup]bool{},
	})
	tools = append(tools,
		chattool.ReadSkill(chattool.ReadSkillOptions{}),
		chattool.ReadSkillFile(chattool.ReadSkillOptions{}),
		chattool.ReadMemory(nil),
		chattool.SaveMemory(nil, ""),
		chattool.DeleteMemory(nil, ""),
		chattool.ConsolidateMemory(nil, ""),
		chatadvisor.Tool(chatadvisor.ToolOptions{}),
		chattool.FindTools(chattool.FindToolsOptions{}),
	)
	seen := map[string]bool{}
	for _, tool := range tools {
		name := tool.Info().Name
		seen[name] = true
		_, ok := builtinToolGroups[name]
		require.True(t, ok, "built-in tool %q has no tool group", name)
	}
	for name := range builtinToolGroups {
		require.True(t, seen[name], "builtinToolGroups lists %q, which turn preparation never adds", name)
	}

	enabled := true
	providerTools := buildProviderTools(&codersdk.ChatModelProviderOptions{
		OpenAI:    &codersdk.ChatModelOpenAIProviderOptions{WebSearchEnabled: &enabled},
		Anthropic: &codersdk.ChatModelAnthropicProviderOptions{WebSearchEnabled: &enabled},
		Google:    &codersdk.ChatModelGoogleProviderOptions{WebSearchEnabled: &enabled},
	})
	for _, provider := range []codersdk.ChatComputerUseProvider{
		codersdk.ChatComputerUseProviderAnthropic,
		codersdk.ChatComputerUseProviderOpenAI,
	} {
		var err error
		providerTools, err = appendComputerUseProviderTool(providerTools, computerUseProviderToolOptions{
			provider:      provider,
			isComputerUse: true,
		})
		require.NoError(t, err)
	}
	require.NotEmpty(t, providerTools)
	for _, tool := range providerTools {
		name := providerToolName(tool)
		_, ok := providerToolGroups[name]
		require.True(t, ok, "provider tool %q has no tool group", name)
	}
}

func providerToolName(tool chatloop.ProviderTool) string {
	return tool.Definition.GetName()
}
