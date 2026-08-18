package chatd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type deferredTestAgentTool struct {
	info fantasy.ToolInfo
}

func (t deferredTestAgentTool) Info() fantasy.ToolInfo                   { return t.info }
func (deferredTestAgentTool) ProviderOptions() fantasy.ProviderOptions   { return nil }
func (deferredTestAgentTool) SetProviderOptions(fantasy.ProviderOptions) {}
func (deferredTestAgentTool) Run(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
	return fantasy.NewTextResponse("ok"), nil
}

func testDeferredTool(name, description string, parameters map[string]any) deferredMCPTool {
	return deferredMCPTool{tool: deferredTestAgentTool{info: fantasy.ToolInfo{
		Name: name, Description: description, Parameters: parameters,
	}}}
}

func TestDecideMCPToolSearch(t *testing.T) {
	t.Parallel()
	small := []deferredMCPTool{testDeferredTool("server__small", "small", map[string]any{"value": map[string]any{"type": "string"}})}
	large := []deferredMCPTool{testDeferredTool("server__large", strings.Repeat("large ", 2000), map[string]any{"value": map[string]any{"type": "string"}})}

	tests := []struct {
		name         string
		experiment   bool
		force        bool
		window       int64
		candidates   []deferredMCPTool
		dynamicNames map[string]bool
		want         bool
	}{
		{name: "below", experiment: true, window: 100_000, candidates: small},
		{name: "above", experiment: true, window: 10_000, candidates: large, want: true},
		{name: "forced", experiment: true, force: true, window: 100_000, candidates: small, want: true},
		{name: "experiment off", force: true, window: 10, candidates: large},
		{name: "empty", experiment: true, force: true},
		{name: "collision", experiment: true, force: true, candidates: []deferredMCPTool{testDeferredTool(chattool.FindToolsName, "collision", nil)}},
		{name: "dynamic collision", experiment: true, force: true, candidates: small, dynamicNames: map[string]bool{chattool.FindToolsName: true}},
		{name: "dynamic no collision", experiment: true, force: true, candidates: small, dynamicNames: map[string]bool{"other": true}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, decideMCPToolSearch(mcpToolSearchInput{
				experimentEnabled: tt.experiment,
				forceDefer:        tt.force,
				contextWindow:     tt.window,
				candidates:        tt.candidates,
				dynamicToolNames:  tt.dynamicNames,
			}).apply)
		})
	}
}

func TestDeriveDeferredMCPActivations(t *testing.T) {
	t.Parallel()
	candidates := []deferredMCPTool{
		testDeferredTool("server__first", "first", nil),
		testDeferredTool("server__second", "second", nil),
	}
	findResult, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolResult("call-1", chattool.FindToolsName, []byte(`{"activated":["server__second","disconnected"]}`), false, false),
	})
	require.NoError(t, err)
	directCall, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolCall("call-2", "server__first", []byte(`{}`)),
	})
	require.NoError(t, err)
	malformed, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolResult("call-3", chattool.FindToolsName, []byte(`"not-json"`), false, false),
	})
	require.NoError(t, err)
	rows := []database.ChatMessage{
		{Role: database.ChatMessageRoleTool, Content: findResult, ContentVersion: chatprompt.CurrentContentVersion},
		{Role: database.ChatMessageRoleAssistant, Content: directCall, ContentVersion: chatprompt.CurrentContentVersion},
		{Role: database.ChatMessageRoleTool, Content: malformed, ContentVersion: chatprompt.CurrentContentVersion},
	}
	require.Equal(t, []string{"server__first", "server__second"}, deriveDeferredMCPActivations(rows, candidates, 0),
		"newest activations first")
	require.Equal(t, []string{"server__first"}, deriveDeferredMCPActivations(rows[1:], candidates, 0),
		"activations before a compaction summary are absent from the surviving prompt window")

	firstWeight := estimateDeferredMCPToolTokens(candidates[:1])
	require.Equal(t, []string{"server__first"}, deriveDeferredMCPActivations(rows, candidates, firstWeight),
		"a token budget sheds the least recent activations")
	require.Equal(t, []string{"server__first"}, deriveDeferredMCPActivations(rows, candidates, 0.001),
		"the newest activation survives a budget smaller than its own schema")
}

func TestDeriveDeferredMCPActivationsSameStepDirectCallPriority(t *testing.T) {
	t.Parallel()
	candidates := []deferredMCPTool{
		testDeferredTool("server__direct", "direct", nil),
		testDeferredTool("server__searched", "searched", nil),
	}
	assistantStep, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolCall("call-search", chattool.FindToolsName, []byte(`{"queries":["direct"]}`)),
		codersdk.ChatMessageToolCall("call-direct", "server__direct", []byte(`{}`)),
	})
	require.NoError(t, err)
	searchResult, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolResult("call-search", chattool.FindToolsName, []byte(`{"activated":["server__searched"]}`), false, false),
	})
	require.NoError(t, err)
	rows := []database.ChatMessage{
		{Role: database.ChatMessageRoleAssistant, Content: assistantStep, ContentVersion: chatprompt.CurrentContentVersion},
		{Role: database.ChatMessageRoleTool, Content: searchResult, ContentVersion: chatprompt.CurrentContentVersion},
	}
	require.Equal(t, []string{"server__direct", "server__searched"}, deriveDeferredMCPActivations(rows, candidates, 0),
		"a step's direct calls outrank its own search activations")
	directWeight := estimateDeferredMCPToolTokens(candidates[:1])
	require.Equal(t, []string{"server__direct"}, deriveDeferredMCPActivations(rows, candidates, directWeight),
		"same-step search activations cannot shed a directly invoked tool's schema")
}

func TestDeriveDeferredMCPActivationsErroredDirectCallsActivateLast(t *testing.T) {
	t.Parallel()
	candidates := []deferredMCPTool{
		testDeferredTool("server__errored", "errored", nil),
		testDeferredTool("server__searched", "searched", nil),
	}
	assistantStep, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolCall("call-search", chattool.FindToolsName, []byte(`{"queries":["x"]}`)),
		codersdk.ChatMessageToolCall("call-errored", "server__errored", []byte(`{}`)),
	})
	require.NoError(t, err)
	toolRow, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolResult("call-errored", "server__errored", []byte(`"remote tool error"`), true, false),
		codersdk.ChatMessageToolResult("call-search", chattool.FindToolsName, []byte(`{"activated":["server__searched"]}`), false, false),
	})
	require.NoError(t, err)
	rows := []database.ChatMessage{
		{Role: database.ChatMessageRoleAssistant, Content: assistantStep, ContentVersion: chatprompt.CurrentContentVersion},
		{Role: database.ChatMessageRoleTool, Content: toolRow, ContentVersion: chatprompt.CurrentContentVersion},
	}
	require.Equal(t, []string{"server__searched", "server__errored"}, deriveDeferredMCPActivations(rows, candidates, 0),
		"an errored direct call activates last so the model keeps the schema for a corrected retry")
	searchedWeight := estimateDeferredMCPToolTokens(candidates[1:])
	require.Equal(t, []string{"server__searched"}, deriveDeferredMCPActivations(rows, candidates, searchedWeight),
		"an errored direct call cannot consume budget promised to the search's reported activations")

	erroredCall, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolCall("call-errored", "server__errored", []byte(`{}`)),
	})
	require.NoError(t, err)
	erroredResult, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolResult("call-errored", "server__errored", []byte(`"remote tool error"`), true, false),
	})
	require.NoError(t, err)
	erroredOnly := []database.ChatMessage{
		{Role: database.ChatMessageRoleAssistant, Content: erroredCall, ContentVersion: chatprompt.CurrentContentVersion},
		{Role: database.ChatMessageRoleTool, Content: erroredResult, ContentVersion: chatprompt.CurrentContentVersion},
	}
	require.Equal(t, []string{"server__errored"}, deriveDeferredMCPActivations(erroredOnly, candidates, 0.001),
		"the newest errored call keeps the first-activation allowance when nothing else activates")
}

func TestFlattenMCPParameterText(t *testing.T) {
	t.Parallel()
	text := flattenMCPParameterText(map[string]any{
		"repository": map[string]any{"type": "string", "description": "Repository name"},
	})
	require.Contains(t, text, "repository")
	require.Contains(t, text, "Repository name")
}

type deferredExternalTestTool struct {
	deferredTestAgentTool
	configID uuid.UUID
}

func (t deferredExternalTestTool) MCPServerConfigID() uuid.UUID { return t.configID }

func TestCollectDeferredMCPCandidates(t *testing.T) {
	t.Parallel()
	approvedID := uuid.New()
	unapprovedID := uuid.New()
	external := deferredExternalTestTool{
		deferredTestAgentTool: deferredTestAgentTool{info: fantasy.ToolInfo{Name: "github__create_issue"}},
		configID:              approvedID,
	}
	unapproved := deferredExternalTestTool{
		deferredTestAgentTool: deferredTestAgentTool{info: fantasy.ToolInfo{Name: "linear__create_issue"}},
		configID:              unapprovedID,
	}
	workspace := deferredTestAgentTool{info: fantasy.ToolInfo{Name: "everything__echo"}}
	input := deferredMCPCandidateInput{
		mcpTools:              []fantasy.AgentTool{external, unapproved},
		workspaceMCPTools:     []fantasy.AgentTool{workspace},
		mcpConfigByID:         map[uuid.UUID]database.MCPServerConfig{approvedID: {Slug: "github", Description: "GitHub"}},
		approvedMCPConfigIDs:  map[uuid.UUID]struct{}{approvedID: {}},
		includeWorkspaceTools: true,
	}

	names := func(candidates []deferredMCPTool) []string {
		out := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			out = append(out, candidate.tool.Info().Name)
		}
		return out
	}

	all := collectDeferredMCPCandidates(input)
	require.Equal(t, []string{"github__create_issue", "linear__create_issue", "everything__echo"}, names(all))
	require.Equal(t, "github", all[0].server)
	require.Equal(t, "everything", all[2].server)

	planInput := input
	planInput.planMode = database.NullChatPlanMode{Valid: true, ChatPlanMode: database.ChatPlanModePlan}
	require.Equal(t, []string{"github__create_issue"}, names(collectDeferredMCPCandidates(planInput)),
		"plan mode keeps only approved external tools, matching filterToolsForTurn")

	noWorkspace := input
	noWorkspace.includeWorkspaceTools = false
	require.Equal(t, []string{"github__create_issue", "linear__create_issue"}, names(collectDeferredMCPCandidates(noWorkspace)))

	longServer := strings.Repeat("s", 70)
	truncated := chattool.NewWorkspaceMCPTool(workspacesdk.MCPToolInfo{Name: longServer + "__echo"}, nil, nil)
	require.NotContains(t, truncated.Info().Name, "__",
		"sanitization must drop the separator for this scenario to be meaningful")
	truncatedInput := deferredMCPCandidateInput{
		workspaceMCPTools:     []fantasy.AgentTool{truncated},
		includeWorkspaceTools: true,
	}
	require.Equal(t, longServer, collectDeferredMCPCandidates(truncatedInput)[0].server,
		"the server comes from the unsanitized routing name, not the capped model name")

	padded := chattool.NewWorkspaceMCPTool(workspacesdk.MCPToolInfo{Name: " everything __echo"}, nil, nil)
	paddedInput := deferredMCPCandidateInput{
		workspaceMCPTools:     []fantasy.AgentTool{padded},
		includeWorkspaceTools: true,
	}
	require.Equal(t, "everything", collectDeferredMCPCandidates(paddedInput)[0].server,
		"surrounding whitespace is trimmed so scope matching and catalog display see the canonical name")
}

func TestConfigureDeferredMCPToolSearchGenerationFlows(t *testing.T) {
	t.Parallel()

	hot := deferredTestAgentTool{info: fantasy.ToolInfo{Name: "read_file", Description: "Read a file"}}
	first := testDeferredTool("github__create_issue", "Create an issue", nil)
	second := testDeferredTool("github__list_issues", "List issues", nil)
	candidates := []deferredMCPTool{first, second}
	findTools := chattool.FindTools(chattool.FindToolsOptions{Entries: deferredMCPToolEntries(candidates)})
	allTools := []fantasy.AgentTool{hot, first.tool, second.tool}
	allActive := []string{"read_file", first.tool.Info().Name, second.tool.Info().Name}

	ordered, active, allowInactive := configureDeferredMCPToolSearch(allTools, allActive, candidates, findTools, nil)
	require.Equal(t, []string{"read_file", chattool.FindToolsName}, captureWireToolNames(t, ordered, active))
	require.Equal(t, map[string]bool{
		first.tool.Info().Name:  true,
		second.tool.Info().Name: true,
	}, allowInactive)

	result, _ := chattool.SearchTools(deferredMCPToolEntries(candidates), chattool.FindToolsArgs{Names: []string{second.tool.Info().Name}}, chattool.SearchBudget{})
	resultJSON, err := json.Marshal(result)
	require.NoError(t, err)
	resultContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolResult("find-1", chattool.FindToolsName, resultJSON, false, false),
	})
	require.NoError(t, err)
	history := []database.ChatMessage{{
		Role: database.ChatMessageRoleTool, Content: resultContent, ContentVersion: chatprompt.CurrentContentVersion,
	}}
	activations := deriveDeferredMCPActivations(history, candidates, 0)
	require.Equal(t, []string{second.tool.Info().Name}, activations)

	ordered, active, _ = configureDeferredMCPToolSearch(allTools, allActive, candidates, findTools, activations)
	require.Equal(t,
		[]string{"read_file", chattool.FindToolsName, second.tool.Info().Name},
		captureWireToolNames(t, ordered, active),
	)
	// Re-preparing the following turn from the same surviving history produces
	// the same activation set without separate persisted state.
	require.Equal(t, activations, deriveDeferredMCPActivations(history, candidates, 0))
}

func TestConfigureDeferredMCPToolSearchDirectCallAndCompaction(t *testing.T) {
	t.Parallel()

	candidate := testDeferredTool("github__create_issue", "Create an issue", nil)
	candidates := []deferredMCPTool{candidate}
	directCall, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolCall("direct-1", candidate.tool.Info().Name, []byte(`{"title":"bug"}`)),
	})
	require.NoError(t, err)
	preSummary := []database.ChatMessage{{
		Role: database.ChatMessageRoleAssistant, Content: directCall, ContentVersion: chatprompt.CurrentContentVersion,
	}}
	activation := deriveDeferredMCPActivations(preSummary, candidates, 0)
	require.Equal(t, []string{candidate.tool.Info().Name}, activation)

	findTools := chattool.FindTools(chattool.FindToolsOptions{Entries: deferredMCPToolEntries(candidates)})
	ordered, active, _ := configureDeferredMCPToolSearch(
		[]fantasy.AgentTool{candidate.tool},
		[]string{candidate.tool.Info().Name},
		candidates,
		findTools,
		activation,
	)
	require.Equal(t,
		[]string{chattool.FindToolsName, candidate.tool.Info().Name},
		captureWireToolNames(t, ordered, active),
	)

	// Prompt preparation passes only the post-summary history window, so an
	// activation before chat_summarized naturally lapses after compaction.
	require.Empty(t, deriveDeferredMCPActivations(nil, candidates, 0))
}

func TestMCPToolSearchBelowThresholdPreservesWireTools(t *testing.T) {
	t.Parallel()

	hot := deferredTestAgentTool{info: fantasy.ToolInfo{Name: "read_file"}}
	candidate := testDeferredTool("github__list_issues", "List issues", nil)
	tools := []fantasy.AgentTool{hot, candidate.tool}
	active := []string{"read_file", candidate.tool.Info().Name}
	withoutExperiment := captureWireToolNames(t, tools, active)

	decision := decideMCPToolSearch(mcpToolSearchInput{
		experimentEnabled: true,
		contextWindow:     100_000,
		candidates:        []deferredMCPTool{candidate},
	})
	require.False(t, decision.apply)
	require.Equal(t, withoutExperiment, captureWireToolNames(t, tools, active))
}

func TestMCPToolSearchExploreAllowlist(t *testing.T) {
	t.Parallel()

	hot := deferredTestAgentTool{info: fantasy.ToolInfo{Name: "read_file"}}
	external := deferredExternalTestTool{
		deferredTestAgentTool: deferredTestAgentTool{info: fantasy.ToolInfo{Name: "github__list_issues"}},
		configID:              uuid.New(),
	}
	tools := []fantasy.AgentTool{hot, external}
	exploreActive := allowedExploreToolNames(tools)
	require.Equal(t, []string{"read_file", external.Info().Name}, exploreActive)

	candidate := deferredMCPTool{tool: external}
	findTools := chattool.FindTools(chattool.FindToolsOptions{Entries: deferredMCPToolEntries([]deferredMCPTool{candidate})})
	_, deferredActive, _ := configureDeferredMCPToolSearch(tools, exploreActive, []deferredMCPTool{candidate}, findTools, nil)
	require.Equal(t, []string{"read_file", chattool.FindToolsName}, deferredActive)
}

func captureWireToolNames(t *testing.T, tools []fantasy.AgentTool, active []string) []string {
	t.Helper()
	var names []string
	model := &chattest.FakeModel{
		ProviderName: "test",
		ModelName:    "test",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			for _, tool := range call.Tools {
				names = append(names, tool.GetName())
			}
			return func(yield func(fantasy.StreamPart) bool) {
				yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop})
			}, nil
		},
	}
	_, err := chatloop.GenerateAssistant(context.Background(), chatloop.GenerateAssistantOptions{
		Model:       model,
		Tools:       tools,
		ActiveTools: active,
	})
	require.NoError(t, err)
	return names
}
