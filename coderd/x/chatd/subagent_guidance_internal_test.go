package chatd

import (
	"context"
	"slices"
	"strings"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSubagentToolGuidanceAvailability(t *testing.T) {
	t.Parallel()

	const parentMessage = "message_parent"
	names := []string{spawnAgentToolName, "wait_agent", "message_agent", "interrupt_agent", "list_agents", listSubagentModelsToolName, parentMessage}
	for _, tc := range []struct {
		name   string
		active []string
	}{
		{name: "All", active: nil},
		{name: "PlanMode", active: []string{spawnAgentToolName, "wait_agent", "list_agents", listSubagentModelsToolName}},
		{name: "SpawnWithoutLifecycle", active: []string{spawnAgentToolName}},
		{name: "SpawnWithWaitOnly", active: []string{spawnAgentToolName, "wait_agent"}},
		{name: "UnrecognizedToolOnly", active: []string{parentMessage}},
		{name: "NoDelegation", active: []string{"read_file"}},
		{name: "InterruptWithoutResume", active: []string{"interrupt_agent"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var tools []fantasy.AgentTool
			for _, name := range names {
				tools = append(tools, newSubagentTool(name, "Base description.", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
					return fantasy.NewTextResponse("ok"), nil
				}))
			}
			got := withSubagentToolGuidance(tools, tc.active)
			for i, tool := range got {
				for _, name := range names {
					if len(tc.active) != 0 && !slices.Contains(tc.active, name) {
						require.NotContains(t, tool.Info().Description, name)
					}
				}
				require.Equal(t, "Base description.", tools[i].Info().Description, "do not mutate the input tool")
			}
			spawn := findToolByName(got, spawnAgentToolName).Info().Description
			spawnActive := len(tc.active) == 0 || slices.Contains(tc.active, spawnAgentToolName)
			for _, target := range names[1 : len(names)-1] {
				wantReference := spawnActive && (len(tc.active) == 0 || slices.Contains(tc.active, target))
				require.Equal(t, wantReference, strings.Contains(spawn, target), target)
			}
			require.NotContains(t, spawn, parentMessage)

			again := withSubagentToolGuidance(got, tc.active)
			for i := range got {
				require.Same(t, got[i], again[i], "unchanged availability must not duplicate guidance")
			}
			revoked := withSubagentToolGuidance(got, []string{"read_file"})
			for _, tool := range revoked {
				require.Equal(t, "Base description.", tool.Info().Description)
			}
		})
	}
}

func TestSubagentToolGuidancePreservesToolContract(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	var received string
	tool := newSubagentTool(spawnAgentToolName, "Spawn.", func(_ context.Context, args struct {
		Prompt string `json:"prompt"`
	}, _ fantasy.ToolCall,
	) (fantasy.ToolResponse, error) {
		received = args.Prompt
		return fantasy.NewTextResponse(args.Prompt), nil
	})
	wait := newSubagentTool("wait_agent", "Wait.", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
		return fantasy.NewTextResponse("done"), nil
	})
	opts := fantasy.ProviderOptions{fantasyanthropic.Name: &fantasyanthropic.ProviderOptions{}}
	tool.SetProviderOptions(opts)
	got := withSubagentToolGuidance([]fantasy.AgentTool{tool, wait}, nil)[0]
	before, after := tool.Info(), got.Info()
	require.Equal(t, before.Name, after.Name)
	require.Equal(t, before.Parameters, after.Parameters)
	require.Equal(t, before.Required, after.Required)
	require.Equal(t, before.Parallel, after.Parallel)
	require.Equal(t, opts, got.ProviderOptions())
	got.SetProviderOptions(nil)
	require.Nil(t, tool.ProviderOptions())
	response, err := got.Run(ctx, fantasy.ToolCall{Name: spawnAgentToolName, Input: `{"prompt":"keep handler"}`})
	require.NoError(t, err)
	require.Equal(t, "keep handler", received)
	require.Equal(t, "keep handler", response.Content)
}

func TestSubagentToolGuidanceIgnoresExternalTools(t *testing.T) {
	t.Parallel()

	spawn := newSubagentTool(spawnAgentToolName, "Spawn.", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
		return fantasy.NewTextResponse("ok"), nil
	})
	external := newTestMCPAgentTool("message_agent", uuid.New())
	externalSpawn := newTestMCPAgentTool(spawnAgentToolName, uuid.New())
	tools := []fantasy.AgentTool{spawn, external, externalSpawn}
	got := withSubagentToolGuidance(tools, nil)
	require.NotContains(t, got[0].Info().Description, "message_agent")
	require.Same(t, external, got[1])
	require.Same(t, externalSpawn, got[2])
}
