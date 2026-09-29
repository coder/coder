package terraform

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/provisioner/terraform/scriptorder"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestConvertStateRecordsScriptOrderRuntimeTargets(t *testing.T) {
	t.Parallel()

	module := scriptOrderRuntimeBindingTestModule(
		scriptOrderRuntimeBindingTestAgent(
			"coder_agent.main", "main", "agent-id",
		),
		scriptOrderRuntimeBindingTestAgent(
			"coder_agent.detached", "detached", "detached-id",
		),
		scriptOrderRuntimeBindingTestDevcontainer(
			"coder_devcontainer.repo", "repo", "agent-id", "subagent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.work", "work", "agent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.prepare", "prepare", "agent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.unselected", "unselected", "agent-id",
		),
	)
	config := &tfjson.Config{RootModule: &tfjson.ConfigModule{
		Resources: []*tfjson.ConfigResource{
			{
				Address: "data.coder_script_order.order",
				Mode:    tfjson.DataResourceMode,
				Type:    "coder_script_order",
				Name:    "order",
			},
			{
				Address: "coder_script.work",
				Mode:    tfjson.ManagedResourceMode,
				Type:    "coder_script",
				Name:    "work",
			},
			{
				Address: "coder_script.prepare",
				Mode:    tfjson.ManagedResourceMode,
				Type:    "coder_script",
				Name:    "prepare",
			},
			{
				Address: "coder_script.unselected",
				Mode:    tfjson.ManagedResourceMode,
				Type:    "coder_script",
				Name:    "unselected",
			},
		},
	}}
	program, err := scriptorder.NewProgram(
		t.Context(), []*tfjson.StateModule{module}, config,
	)
	require.NoError(t, err)
	require.NotNil(t, program)

	conversion, err := convertState(
		t.Context(),
		[]*tfjson.StateModule{module},
		scriptOrderRuntimeBindingTestConversionGraph(),
		slogtest.Make(t, nil),
		&scriptOrderRuntimeBindingInput{program: program},
	)
	require.NoError(t, err)
	require.NotNil(t, conversion.scriptOrder)
	require.ElementsMatch(
		t,
		[]string{"coder_script.prepare", "coder_script.work"},
		conversion.scriptOrder.prepared.SelectedScriptAddresses(),
	)
	require.Len(t, conversion.scriptOrder.scriptRecords, 3)
	require.True(
		t,
		conversion.scriptOrder.scriptRecords["coder_script.work"].selected,
	)
	require.True(
		t,
		conversion.scriptOrder.scriptRecords["coder_script.prepare"].selected,
	)
	require.False(
		t,
		conversion.scriptOrder.scriptRecords["coder_script.unselected"].selected,
	)
	require.True(
		t,
		conversion.scriptOrder.scripts["coder_script.work"].RunOnStart,
	)

	agents := scriptOrderRuntimeBindingTestAgents(conversion.state)
	require.Len(t, agents, 1)
	require.Equal(t, "main", agents[0].Name)
	require.Len(t, agents[0].Devcontainers, 1)

	require.Len(t, conversion.scriptOrder.runtimeTargets, 2)
	require.Same(
		t,
		agents[0],
		conversion.scriptOrder.runtimeTargets["coder_agent.main"].workspaceAgent,
	)
	require.Same(
		t,
		agents[0].Devcontainers[0],
		conversion.scriptOrder.runtimeTargets["coder_devcontainer.repo"].devcontainer,
	)
	require.NotContains(
		t, conversion.scriptOrder.runtimeTargets, "coder_agent.detached",
	)
}

func scriptOrderRuntimeBindingTestModule(
	resources ...*tfjson.StateResource,
) *tfjson.StateModule {
	resources = append(resources, &tfjson.StateResource{
		Address: "docker_container.workspace",
		Mode:    tfjson.ManagedResourceMode,
		Type:    "docker_container",
		Name:    "workspace",
	})
	resources = append(resources, &tfjson.StateResource{
		Address: "data.coder_script_order.order",
		Mode:    tfjson.DataResourceMode,
		Type:    "coder_script_order",
		Name:    "order",
		AttributeValues: map[string]any{
			"rule": []any{map[string]any{
				"run":   []string{"coder_script.work"},
				"after": []string{"coder_script.prepare"},
			}},
		},
	})
	return &tfjson.StateModule{Resources: resources}
}

func scriptOrderRuntimeBindingTestAgent(
	address string,
	name string,
	id string,
) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.ManagedResourceMode,
		Type:    "coder_agent",
		Name:    name,
		AttributeValues: map[string]any{
			"arch": "amd64",
			"id":   id,
		},
	}
}

func scriptOrderRuntimeBindingTestDevcontainer(
	address string,
	name string,
	agentID string,
	subagentID string,
) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.ManagedResourceMode,
		Type:    "coder_devcontainer",
		Name:    name,
		AttributeValues: map[string]any{
			"agent_id":         agentID,
			"subagent_id":      subagentID,
			"workspace_folder": "/workspace",
		},
	}
}

func scriptOrderRuntimeBindingTestScript(
	address string,
	name string,
	agentID string,
) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.ManagedResourceMode,
		Type:    "coder_script",
		Name:    name,
		AttributeValues: map[string]any{
			"agent_id":     agentID,
			"run_on_start": true,
		},
	}
}

func scriptOrderRuntimeBindingTestConversionGraph() string {
	return `digraph {
		"[root] docker_container.workspace" [label = "docker_container.workspace"]
		"[root] coder_agent.detached (expand)" [label = "coder_agent.detached"]
		"[root] coder_agent.main (expand)" [label = "coder_agent.main"]
		"[root] docker_container.workspace" -> "[root] coder_agent.main (expand)"
	}`
}

func scriptOrderRuntimeBindingTestAgents(state *State) []*proto.Agent {
	var agents []*proto.Agent
	for _, resource := range state.Resources {
		agents = append(agents, resource.Agents...)
	}
	return agents
}
