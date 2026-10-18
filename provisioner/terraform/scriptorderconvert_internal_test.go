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

func TestConvertStateWithScriptOrderAttachesDependencies(t *testing.T) {
	t.Parallel()

	module := scriptOrderTestModule(
		scriptOrderTestAgent("coder_agent.main", "main", "agent-id"),
		scriptOrderTestDevcontainer(
			"coder_devcontainer.repo", "repo", "agent-id", "subagent-id",
		),
		scriptOrderTestScript("coder_script.clone_repo", "clone_repo", "agent-id"),
		scriptOrderTestScript("coder_script.install_tools", "install_tools", "agent-id"),
		scriptOrderTestScript("coder_script.build", "build", "agent-id"),
		scriptOrderTestScript("coder_script.dc_setup", "dc_setup", "subagent-id"),
		scriptOrderTestScript("coder_script.dc_run", "dc_run", "subagent-id"),
	)
	module.Resources[len(module.Resources)-1].AttributeValues = map[string]any{
		"rule": []any{
			map[string]any{
				"run":   []string{"coder_script.install_tools"},
				"after": []string{"coder_script.clone_repo"},
			},
			map[string]any{
				// Prerequisites are listed out of order on purpose.
				"run":      []string{"coder_script.build"},
				"after":    []string{"coder_script.install_tools", "coder_script.clone_repo"},
				"requires": "completion",
			},
			map[string]any{
				"run":   []string{"coder_script.dc_run"},
				"after": []string{"coder_script.dc_setup"},
			},
		},
	}
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

	var agents []*proto.Agent
	for _, resource := range conversion.state.Resources {
		agents = append(agents, resource.Agents...)
	}
	require.Len(t, agents, 1)
	agent := agents[0]
	require.Len(t, agent.Devcontainers, 1)
	agentScripts := scriptOrderTestScriptsByAddress(t, agent.Scripts,
		"coder_script.build", "coder_script.clone_repo", "coder_script.install_tools",
	)
	devcontainerScripts := scriptOrderTestScriptsByAddress(t, agent.Devcontainers[0].Scripts,
		"coder_script.dc_run", "coder_script.dc_setup",
	)

	success := proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS
	completion := proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_COMPLETION
	require.Empty(t, agentScripts["coder_script.clone_repo"].Dependencies)
	require.Equal(t, []*proto.ScriptDependency{
		{PrerequisiteResourceAddress: "coder_script.clone_repo", Requirement: success},
	}, agentScripts["coder_script.install_tools"].Dependencies)
	require.Equal(t, []*proto.ScriptDependency{
		{PrerequisiteResourceAddress: "coder_script.clone_repo", Requirement: completion},
		{PrerequisiteResourceAddress: "coder_script.install_tools", Requirement: completion},
	}, agentScripts["coder_script.build"].Dependencies)
	require.Empty(t, devcontainerScripts["coder_script.dc_setup"].Dependencies)
	require.Equal(t, []*proto.ScriptDependency{
		{PrerequisiteResourceAddress: "coder_script.dc_setup", Requirement: success},
	}, devcontainerScripts["coder_script.dc_run"].Dependencies)
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

func TestHasScriptOrderDataSource(t *testing.T) {
	t.Parallel()

	dataSource := func(address string) *tfjson.StateResource {
		return &tfjson.StateResource{
			Address: address,
			Mode:    tfjson.DataResourceMode,
			Type:    "coder_script_order",
			Name:    "order",
		}
	}
	tests := []struct {
		name    string
		modules []*tfjson.StateModule
		want    bool
	}{
		{
			name: "RootModule",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					dataSource("data.coder_script_order.order"),
				},
			}},
			want: true,
		},
		{
			name: "ChildModule",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{{
					Address: "module.child",
					Resources: []*tfjson.StateResource{
						dataSource("module.child.data.coder_script_order.order"),
					},
				}},
			}},
			want: true,
		},
		{
			name: "WrongMode",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{{
					Mode: tfjson.ManagedResourceMode,
					Type: "coder_script_order",
				}},
			}},
		},
		{
			name: "WrongType",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{{
					Mode: tfjson.DataResourceMode,
					Type: "coder_parameter",
				}},
			}},
		},
		{
			name: "NilEntries",
			modules: []*tfjson.StateModule{
				nil,
				{
					Resources:    []*tfjson.StateResource{nil},
					ChildModules: []*tfjson.StateModule{nil},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, hasScriptOrderDataSource(test.modules))
		})
	}
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

// scriptOrderTestScriptsByAddress checks that scripts carry exactly the given
// resource addresses and returns them keyed by address.
func scriptOrderTestScriptsByAddress(
	t *testing.T,
	scripts []*proto.Script,
	addresses ...string,
) map[string]*proto.Script {
	t.Helper()

	byAddress := make(map[string]*proto.Script, len(scripts))
	got := make([]string, 0, len(scripts))
	for _, script := range scripts {
		byAddress[script.ResourceAddress] = script
		got = append(got, script.ResourceAddress)
	}
	require.ElementsMatch(t, addresses, got)
	return byAddress
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
