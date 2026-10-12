package terraform

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/provisioner/terraform/scriptorder"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestConvertStateWithScriptOrderFinalizesAppliedOrder(t *testing.T) {
	t.Parallel()

	module := scriptOrderTestModule(
		scriptOrderTestAgent("coder_agent.main", "main", "agent-id"),
		scriptOrderTestScript("coder_script.dependent", "dependent", "agent-id"),
		scriptOrderTestScript("coder_script.prerequisite", "prerequisite", "agent-id"),
	)
	conversion, err := convertStateWithScriptOrder(
		t.Context(),
		[]*tfjson.StateModule{module},
		scriptOrderTestConfig(module),
		scriptOrderTestGraph("coder_agent.main"),
		slogtest.Make(t, nil),
		proto.GraphSource_SOURCE_STATE,
	)
	require.NoError(t, err)
	require.NotNil(t, conversion.state)
	require.Equal(t, &scriptorder.Order{Graphs: []scriptorder.Graph{{
		RuntimeAddress: "coder_agent.main",
		Phase:          scriptorder.PhaseStart,
		Dependencies: []scriptorder.Dependency{{
			DependentAddress:    "coder_script.dependent",
			PrerequisiteAddress: "coder_script.prerequisite",
			Requirement:         scriptorder.RequirementSuccess,
		}},
	}}}, conversion.order)
}

func TestConvertStateWithScriptOrderFinalizesAppliedDevcontainerOrder(t *testing.T) {
	t.Parallel()

	module := scriptOrderTestModule(
		scriptOrderTestAgent("coder_agent.main", "main", "agent-id"),
		scriptOrderTestDevcontainer(
			"coder_devcontainer.repo", "repo", "agent-id", "subagent-id",
		),
		scriptOrderTestScript("coder_script.dependent", "dependent", "subagent-id"),
		scriptOrderTestScript("coder_script.prerequisite", "prerequisite", "subagent-id"),
	)
	conversion, err := convertStateWithScriptOrder(
		t.Context(),
		[]*tfjson.StateModule{module},
		scriptOrderTestConfig(module),
		scriptOrderTestGraph("coder_agent.main"),
		slogtest.Make(t, nil),
		proto.GraphSource_SOURCE_STATE,
	)
	require.NoError(t, err)
	require.NotNil(t, conversion.order)
	require.Len(t, conversion.order.Graphs, 1)
	require.Equal(t, "coder_devcontainer.repo", conversion.order.Graphs[0].RuntimeAddress)
}

func TestConvertStateWithScriptOrderValidatesPlanWithoutAgentBindings(t *testing.T) {
	t.Parallel()

	module := scriptOrderTestModule(
		scriptOrderTestAgent("coder_agent.main", "main", ""),
		scriptOrderTestScript("coder_script.dependent", "dependent", ""),
		scriptOrderTestScript("coder_script.prerequisite", "prerequisite", ""),
	)
	conversion, err := convertStateWithScriptOrder(
		t.Context(),
		[]*tfjson.StateModule{module},
		scriptOrderTestConfig(module),
		scriptOrderTestGraph("coder_agent.main"),
		slogtest.Make(t, nil),
		proto.GraphSource_SOURCE_PLAN,
	)
	require.NoError(t, err)
	require.NotNil(t, conversion.state)
	require.Nil(t, conversion.order)
}

func TestConvertStateWithScriptOrderRejectsPlanSelfDependency(t *testing.T) {
	t.Parallel()

	module := scriptOrderTestModule(
		scriptOrderTestAgent("coder_agent.main", "main", ""),
		scriptOrderTestScript("coder_script.dependent", "dependent", ""),
	)
	module.Resources[len(module.Resources)-1].AttributeValues = map[string]any{
		"rule": []any{map[string]any{
			"run":   []string{"coder_script.dependent"},
			"after": []string{"coder_script.dependent"},
		}},
	}
	_, err := convertStateWithScriptOrder(
		t.Context(),
		[]*tfjson.StateModule{module},
		scriptOrderTestConfig(module),
		scriptOrderTestGraph("coder_agent.main"),
		slogtest.Make(t, nil),
		proto.GraphSource_SOURCE_PLAN,
	)
	require.ErrorContains(t, err, "validate script order for plan")
	require.ErrorContains(t, err, "a script cannot depend on itself")
}

func TestConvertStateWithScriptOrderRejectsAppliedCrossAgentRule(t *testing.T) {
	t.Parallel()

	module := scriptOrderTestModule(
		scriptOrderTestAgent("coder_agent.first", "first", "first-id"),
		scriptOrderTestAgent("coder_agent.second", "second", "second-id"),
		scriptOrderTestScript("coder_script.dependent", "dependent", "first-id"),
		scriptOrderTestScript("coder_script.prerequisite", "prerequisite", "second-id"),
	)
	_, err := convertStateWithScriptOrder(
		t.Context(),
		[]*tfjson.StateModule{module},
		scriptOrderTestConfig(module),
		scriptOrderTestGraph("coder_agent.first", "coder_agent.second"),
		slogtest.Make(t, nil),
		proto.GraphSource_SOURCE_STATE,
	)
	require.ErrorContains(t, err, "scripts can be ordered only within the same agent or devcontainer subagent")
}

func TestConvertStateWithScriptOrderPropagatesAppliedBindingError(t *testing.T) {
	t.Parallel()

	module := scriptOrderTestModule(
		scriptOrderTestAgent("coder_agent.main", "main", "agent-id"),
		scriptOrderTestScript("coder_script.dependent", "dependent", "missing-id"),
		scriptOrderTestScript("coder_script.prerequisite", "prerequisite", "agent-id"),
	)
	_, err := convertStateWithScriptOrder(
		t.Context(),
		[]*tfjson.StateModule{module},
		scriptOrderTestConfig(module),
		scriptOrderTestGraph("coder_agent.main"),
		slogtest.Make(t, nil),
		proto.GraphSource_SOURCE_STATE,
	)
	require.ErrorContains(t, err, "does not match any workspace agent or devcontainer subagent")
}

func TestConvertStateWithScriptOrderRejectsAgentOmittedFromState(t *testing.T) {
	t.Parallel()

	module := scriptOrderTestModule(
		scriptOrderTestAgent("coder_agent.main", "main", "agent-id"),
		scriptOrderTestScript("coder_script.dependent", "dependent", "agent-id"),
		scriptOrderTestScript("coder_script.prerequisite", "prerequisite", "agent-id"),
	)
	const graph = `digraph {
		"[root] docker_container.workspace" [label = "docker_container.workspace"]
		"[root] coder_agent.main (expand)" [label = "coder_agent.main"]
	}`
	_, err := convertStateWithScriptOrder(
		t.Context(),
		[]*tfjson.StateModule{module},
		scriptOrderTestConfig(module),
		graph,
		slogtest.Make(t, nil),
		proto.GraphSource_SOURCE_STATE,
	)
	require.ErrorContains(t, err, "does not match any workspace agent or devcontainer subagent")
}

func TestConvertStateWithScriptOrderWithoutDataSource(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		{Address: "docker_container.workspace", Mode: tfjson.ManagedResourceMode, Type: "docker_container", Name: "workspace"},
		scriptOrderTestAgent("coder_agent.main", "main", "agent-id"),
	}}
	conversion, err := convertStateWithScriptOrder(
		t.Context(),
		[]*tfjson.StateModule{module},
		nil,
		scriptOrderTestGraph("coder_agent.main"),
		slogtest.Make(t, nil),
		proto.GraphSource_SOURCE_STATE,
	)
	require.NoError(t, err)
	require.NotNil(t, conversion.state)
	require.Nil(t, conversion.order)
	require.Empty(t, conversion.warnings)
}

func TestConvertStateWithScriptOrderRejectsUnknownSource(t *testing.T) {
	t.Parallel()

	_, err := convertStateWithScriptOrder(
		t.Context(), nil, nil, "digraph {}", slogtest.Make(t, nil),
		proto.GraphSource(99),
	)
	require.ErrorContains(t, err, "unknown script order graph source 99")
}

func scriptOrderTestModule(resources ...*tfjson.StateResource) *tfjson.StateModule {
	resources = append(resources,
		&tfjson.StateResource{
			Address: "docker_container.workspace",
			Mode:    tfjson.ManagedResourceMode,
			Type:    "docker_container",
			Name:    "workspace",
		},
		&tfjson.StateResource{
			Address: "data.coder_script_order.order",
			Mode:    tfjson.DataResourceMode,
			Type:    "coder_script_order",
			Name:    "order",
			AttributeValues: map[string]any{
				"rule": []any{map[string]any{
					"run":   []string{"coder_script.dependent"},
					"after": []string{"coder_script.prerequisite"},
				}},
			},
		},
	)
	return &tfjson.StateModule{Resources: resources}
}

func scriptOrderTestConfig(module *tfjson.StateModule) *tfjson.Config {
	resources := make([]*tfjson.ConfigResource, 0, len(module.Resources))
	for _, resource := range module.Resources {
		if resource.Type != "coder_script" && resource.Type != "coder_script_order" {
			continue
		}
		resources = append(resources, &tfjson.ConfigResource{
			Address: resource.Address,
			Mode:    resource.Mode,
			Type:    resource.Type,
			Name:    resource.Name,
		})
	}
	return &tfjson.Config{RootModule: &tfjson.ConfigModule{Resources: resources}}
}

func scriptOrderTestGraph(agentAddresses ...string) string {
	graph := "digraph {\n"
	graph += `"[root] docker_container.workspace" [label = "docker_container.workspace"]` + "\n"
	for _, address := range agentAddresses {
		graph += `"[root] ` + address + ` (expand)" [label = "` + address + `"]` + "\n"
		graph += `"[root] docker_container.workspace" -> "[root] ` + address + ` (expand)"` + "\n"
	}
	return graph + "}\n"
}
