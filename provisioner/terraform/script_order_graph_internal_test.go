package terraform

import (
	"context"
	"fmt"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestBuildScriptOrderWithoutDataSources(t *testing.T) {
	t.Parallel()

	order, err := buildScriptOrder([]*tfjson.StateModule{{}}, nil)
	require.NoError(t, err)
	require.Empty(t, order)
}

func TestBuildScriptOrderMergeAndDeduplication(t *testing.T) {
	t.Parallel()

	commonResources := scriptOrderTestManagedScripts("a", "b", "c", "d", "unordered")
	commonScripts := scriptOrderTestScripts("coder_agent.main", ScriptOrderPhaseStartup,
		"coder_script.a", "coder_script.b", "coder_script.c", "coder_script.d", "coder_script.unordered")

	tests := []struct {
		name             string
		resources        []*tfjson.StateResource
		scripts          map[string]scriptOrderScript
		dataSourceRules  [][]scriptOrderTestRule
		wantScripts      []string
		wantDependencies []ScriptOrderDependency
		wantError        []string
	}{
		{
			name: "1EachScriptRunsOncePerGraph",
			dataSourceRules: [][]scriptOrderTestRule{{
				{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}},
				{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}},
			}},
			wantScripts: []string{"coder_script.a", "coder_script.b"},
			wantDependencies: []ScriptOrderDependency{
				scriptOrderTestDependency("coder_script.b", "coder_script.a", ScriptOrderRequirementSuccess),
			},
		},
		{
			name: "2DependenciesUnionAcrossRulesAndDataSources",
			dataSourceRules: [][]scriptOrderTestRule{
				{{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}}},
				{{run: []string{"coder_script.b"}, after: []string{"coder_script.c"}}},
			},
			wantScripts: []string{"coder_script.a", "coder_script.b", "coder_script.c"},
			wantDependencies: []ScriptOrderDependency{
				scriptOrderTestDependency("coder_script.b", "coder_script.a", ScriptOrderRequirementSuccess),
				scriptOrderTestDependency("coder_script.b", "coder_script.c", ScriptOrderRequirementSuccess),
			},
		},
		{
			name: "3EveryRunDependsOnEveryAfter",
			dataSourceRules: [][]scriptOrderTestRule{{{
				run:      []string{"coder_script.c", "coder_script.b"},
				after:    []string{"coder_script.d", "coder_script.a"},
				requires: "completion",
			}}},
			wantScripts: []string{"coder_script.a", "coder_script.b", "coder_script.c", "coder_script.d"},
			wantDependencies: []ScriptOrderDependency{
				scriptOrderTestDependency("coder_script.b", "coder_script.a", ScriptOrderRequirementCompletion),
				scriptOrderTestDependency("coder_script.b", "coder_script.d", ScriptOrderRequirementCompletion),
				scriptOrderTestDependency("coder_script.c", "coder_script.a", ScriptOrderRequirementCompletion),
				scriptOrderTestDependency("coder_script.c", "coder_script.d", ScriptOrderRequirementCompletion),
			},
		},
		{
			name: "4SelectorListOrderHasNoExecutionMeaning",
			dataSourceRules: [][]scriptOrderTestRule{{{
				run:   []string{"coder_script.c", "coder_script.b"},
				after: []string{"coder_script.a"},
			}}},
			wantScripts: []string{"coder_script.a", "coder_script.b", "coder_script.c"},
			wantDependencies: []ScriptOrderDependency{
				scriptOrderTestDependency("coder_script.b", "coder_script.a", ScriptOrderRequirementSuccess),
				scriptOrderTestDependency("coder_script.c", "coder_script.a", ScriptOrderRequirementSuccess),
			},
		},
		{
			name: "5MultipleRunScriptsRemainParallel",
			dataSourceRules: [][]scriptOrderTestRule{{{
				run:   []string{"coder_script.b", "coder_script.c"},
				after: []string{"coder_script.a"},
			}}},
			wantScripts: []string{"coder_script.a", "coder_script.b", "coder_script.c"},
			wantDependencies: []ScriptOrderDependency{
				scriptOrderTestDependency("coder_script.b", "coder_script.a", ScriptOrderRequirementSuccess),
				scriptOrderTestDependency("coder_script.c", "coder_script.a", ScriptOrderRequirementSuccess),
			},
		},
		{
			name: "6IndependentGroupsAndUnorderedScriptsRemainParallel",
			dataSourceRules: [][]scriptOrderTestRule{{
				{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}},
				{run: []string{"coder_script.d"}, after: []string{"coder_script.c"}},
			}},
			wantScripts: []string{"coder_script.a", "coder_script.b", "coder_script.c", "coder_script.d"},
			wantDependencies: []ScriptOrderDependency{
				scriptOrderTestDependency("coder_script.b", "coder_script.a", ScriptOrderRequirementSuccess),
				scriptOrderTestDependency("coder_script.d", "coder_script.c", ScriptOrderRequirementSuccess),
			},
		},
		{
			name: "7DuplicateRunScriptsAreDeduplicated",
			dataSourceRules: [][]scriptOrderTestRule{{{
				run:   []string{"coder_script.b", "coder_script.b"},
				after: []string{"coder_script.a"},
			}}},
			wantScripts: []string{"coder_script.a", "coder_script.b"},
			wantDependencies: []ScriptOrderDependency{
				scriptOrderTestDependency("coder_script.b", "coder_script.a", ScriptOrderRequirementSuccess),
			},
		},
		{
			name: "8DuplicateAfterScriptsAreDeduplicated",
			dataSourceRules: [][]scriptOrderTestRule{{{
				run:   []string{"coder_script.b"},
				after: []string{"coder_script.a", "coder_script.a"},
			}}},
			wantScripts: []string{"coder_script.a", "coder_script.b"},
			wantDependencies: []ScriptOrderDependency{
				scriptOrderTestDependency("coder_script.b", "coder_script.a", ScriptOrderRequirementSuccess),
			},
		},
		{
			name: "9OverlappingSelectorsAreDeduplicated",
			resources: []*tfjson.StateResource{
				managedScript("coder_script.a", "a"),
				managedScript("coder_script.setup[0]", "setup"),
				managedScript("coder_script.setup[1]", "setup"),
			},
			scripts: scriptOrderTestScripts("coder_agent.main", ScriptOrderPhaseStartup,
				"coder_script.a", "coder_script.setup[0]", "coder_script.setup[1]"),
			dataSourceRules: [][]scriptOrderTestRule{{{
				run:   []string{"coder_script.setup", "coder_script.setup[0]"},
				after: []string{"coder_script.a"},
			}}},
			wantScripts: []string{"coder_script.a", "coder_script.setup[0]", "coder_script.setup[1]"},
			wantDependencies: []ScriptOrderDependency{
				scriptOrderTestDependency("coder_script.setup[0]", "coder_script.a", ScriptOrderRequirementSuccess),
				scriptOrderTestDependency("coder_script.setup[1]", "coder_script.a", ScriptOrderRequirementSuccess),
			},
		},
		{
			name: "10IdenticalEdgesAcrossDataSourcesAreDeduplicated",
			dataSourceRules: [][]scriptOrderTestRule{
				{{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}}},
				{{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}}},
			},
			wantScripts: []string{"coder_script.a", "coder_script.b"},
			wantDependencies: []ScriptOrderDependency{
				scriptOrderTestDependency("coder_script.b", "coder_script.a", ScriptOrderRequirementSuccess),
			},
		},
		{
			name: "11ConflictingRequirementsAcrossDataSourcesFail",
			dataSourceRules: [][]scriptOrderTestRule{
				{{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}, requires: "success"}},
				{{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}, requires: "completion"}},
			},
			wantError: []string{"data.coder_script_order.order_1", "rule 0", "coder_script.b", "coder_script.a", "conflicting with requires"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resources := test.resources
			if resources == nil {
				resources = commonResources
			}
			scripts := test.scripts
			if scripts == nil {
				scripts = commonScripts
			}
			module := &tfjson.StateModule{Resources: append([]*tfjson.StateResource{}, resources...)}
			for index, rules := range test.dataSourceRules {
				name := fmt.Sprintf("order_%d", index)
				module.Resources = append(module.Resources, scriptOrderTestDataSource(
					"data.coder_script_order."+name, name, rules...,
				))
			}

			order, err := buildScriptOrder([]*tfjson.StateModule{module}, scripts)
			if len(test.wantError) > 0 {
				require.Error(t, err)
				require.Empty(t, order)
				for _, contains := range test.wantError {
					require.ErrorContains(t, err, contains)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, []ScriptOrderGraph{{
				RuntimeAddress: "coder_agent.main",
				Phase:          ScriptOrderPhaseStartup,
				Scripts:        test.wantScripts,
				Dependencies:   test.wantDependencies,
			}}, order.Graphs)
		})
	}
}

func TestBuildScriptOrderSeparatesRuntimeAndPhaseGraphs(t *testing.T) {
	t.Parallel()

	resources := scriptOrderTestManagedScripts("start_a", "start_b", "stop_a", "stop_b", "dc_a", "dc_b")
	resources = append(resources, scriptOrderTestDataSource("data.coder_script_order.order", "order",
		scriptOrderTestRule{run: []string{"coder_script.start_b"}, after: []string{"coder_script.start_a"}},
		scriptOrderTestRule{run: []string{"coder_script.stop_b"}, after: []string{"coder_script.stop_a"}},
		scriptOrderTestRule{run: []string{"coder_script.dc_b"}, after: []string{"coder_script.dc_a"}},
	))
	scripts := scriptOrderTestScripts("coder_agent.main", ScriptOrderPhaseStartup,
		"coder_script.start_a", "coder_script.start_b")
	for address, script := range scriptOrderTestScripts("coder_agent.main", ScriptOrderPhaseShutdown,
		"coder_script.stop_a", "coder_script.stop_b") {
		scripts[address] = script
	}
	for address, script := range scriptOrderTestScripts("coder_devcontainer.dev", ScriptOrderPhaseStartup,
		"coder_script.dc_a", "coder_script.dc_b") {
		scripts[address] = script
	}

	order, err := buildScriptOrder([]*tfjson.StateModule{{Resources: resources}}, scripts)
	require.NoError(t, err)
	require.Equal(t, []ScriptOrderGraph{
		scriptOrderTestGraph("coder_agent.main", ScriptOrderPhaseShutdown, "coder_script.stop_a", "coder_script.stop_b"),
		scriptOrderTestGraph("coder_agent.main", ScriptOrderPhaseStartup, "coder_script.start_a", "coder_script.start_b"),
		scriptOrderTestGraph("coder_devcontainer.dev", ScriptOrderPhaseStartup, "coder_script.dc_a", "coder_script.dc_b"),
	}, order.Graphs)
}

func TestBuildScriptOrderRejectsCycles(t *testing.T) {
	t.Parallel()

	resources := scriptOrderTestManagedScripts("a", "b")
	resources = append(resources, scriptOrderTestDataSource("data.coder_script_order.order", "order",
		scriptOrderTestRule{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}},
		scriptOrderTestRule{run: []string{"coder_script.a"}, after: []string{"coder_script.b"}},
	))
	scripts := scriptOrderTestScripts("coder_agent.main", ScriptOrderPhaseStartup, "coder_script.a", "coder_script.b")

	order, err := buildScriptOrder([]*tfjson.StateModule{{Resources: resources}}, scripts)
	require.Error(t, err)
	require.Empty(t, order)
	for _, contains := range []string{"data.coder_script_order.order", "rule 0", "coder_script.a", "coder_script.b", "dependency cycle"} {
		require.ErrorContains(t, err, contains)
	}
}

func TestBuildScriptOrderDeterministic(t *testing.T) {
	t.Parallel()

	resources := scriptOrderTestManagedScripts("a", "b", "c", "d")
	resources = append(resources, scriptOrderTestDataSource("data.coder_script_order.order", "order",
		scriptOrderTestRule{run: []string{"coder_script.d", "coder_script.c"}, after: []string{"coder_script.b", "coder_script.a"}},
	))
	modules := []*tfjson.StateModule{{Resources: resources}}
	scripts := scriptOrderTestScripts("coder_agent.main", ScriptOrderPhaseStartup,
		"coder_script.a", "coder_script.b", "coder_script.c", "coder_script.d")

	first, err := buildScriptOrder(modules, scripts)
	require.NoError(t, err)
	for range 10 {
		order, err := buildScriptOrder(modules, scripts)
		require.NoError(t, err)
		require.Equal(t, first, order)
	}
}

func TestConvertStateBuildsScriptOrderForDevcontainerSubagent(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		{
			Address: "coder_agent.main",
			Mode:    tfjson.ManagedResourceMode,
			Type:    "coder_agent",
			Name:    "main",
			AttributeValues: map[string]any{
				"id":   "agent-id",
				"arch": "amd64",
			},
		},
		{
			Address: "coder_devcontainer.dev",
			Mode:    tfjson.ManagedResourceMode,
			Type:    "coder_devcontainer",
			Name:    "dev",
			AttributeValues: map[string]any{
				"agent_id":         "agent-id",
				"subagent_id":      "subagent-id",
				"workspace_folder": "/workspace",
			},
		},
		scriptOrderStateScript("coder_script.a", "a", "subagent-id", true, false),
		scriptOrderStateScript("coder_script.b", "b", "subagent-id", true, false),
		scriptOrderTestDataSource("data.coder_script_order.order", "order",
			scriptOrderTestRule{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}},
		),
		{
			Address: "null_resource.workspace",
			Mode:    tfjson.ManagedResourceMode,
			Type:    "null_resource",
			Name:    "workspace",
		},
	}}
	graph := `digraph {
		"[root] null_resource.workspace" [label = "null_resource.workspace", shape = "box"]
		"[root] coder_agent.main" [label = "coder_agent.main", shape = "box"]
		"[root] null_resource.workspace" -> "[root] coder_agent.main"
	}`

	state, err := ConvertState(context.Background(), []*tfjson.StateModule{module}, graph, slogtest.Make(t, nil))
	require.NoError(t, err)
	require.Equal(t, &ScriptOrder{
		Graphs: []ScriptOrderGraph{
			scriptOrderTestGraph("coder_devcontainer.dev", ScriptOrderPhaseStartup, "coder_script.a", "coder_script.b"),
		},
	}, state.ScriptOrder)

	var scripts []*proto.Script
	for _, resource := range state.Resources {
		for _, agent := range resource.Agents {
			for _, devcontainer := range agent.Devcontainers {
				scripts = append(scripts, devcontainer.Scripts...)
			}
		}
	}
	require.Len(t, scripts, 2)
	require.Equal(t, "coder_script.a", scripts[0].ResourceAddress)
	require.Empty(t, scripts[0].Dependencies)
	require.Equal(t, "coder_script.b", scripts[1].ResourceAddress)
	require.Equal(t, []*proto.ScriptDependency{{
		PrerequisiteResourceAddress: "coder_script.a",
		Requirement:                 proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS,
	}}, scripts[1].Dependencies)
}

func TestScriptDependencyRequirementProto(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		requirement ScriptOrderRequirement
		want        proto.ScriptDependencyRequirement
		wantError   bool
	}{
		{
			name:        "success",
			requirement: ScriptOrderRequirementSuccess,
			want:        proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS,
		},
		{
			name:        "completion",
			requirement: ScriptOrderRequirementCompletion,
			want:        proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_COMPLETION,
		},
		{
			name:        "unknown",
			requirement: "unknown",
			want:        proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_UNSPECIFIED,
			wantError:   true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := scriptDependencyRequirementProto(test.requirement)
			if test.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.want, got)
		})
	}
}

func TestConvertStateBuildsScriptOrderForModuleScriptsFromPlan(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			{
				Address: "coder_agent.main",
				Mode:    tfjson.ManagedResourceMode,
				Type:    "coder_agent",
				Name:    "main",
				AttributeValues: map[string]any{
					"arch": "amd64",
				},
			},
			{
				Address: "null_resource.workspace",
				Mode:    tfjson.ManagedResourceMode,
				Type:    "null_resource",
				Name:    "workspace",
			},
		},
		ChildModules: []*tfjson.StateModule{{
			Address: "module.bootstrap",
			Resources: []*tfjson.StateResource{
				scriptOrderStateScript("module.bootstrap.coder_script.a", "a", "", true, false),
				scriptOrderStateScript("module.bootstrap.coder_script.b", "b", "", true, false),
				scriptOrderTestDataSource("module.bootstrap.data.coder_script_order.order", "order",
					scriptOrderTestRule{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}},
				),
			},
		}},
	}
	graph := `digraph {
		"[root] coder_agent.main (expand)" [label = "coder_agent.main", shape = "box"]
		"[root] module.bootstrap.var.agent_id (expand)" [label = "module.bootstrap.var.agent_id", shape = "box"]
		"[root] module.bootstrap.coder_script.a (expand)" [label = "module.bootstrap.coder_script.a", shape = "box"]
		"[root] module.bootstrap.coder_script.b (expand)" [label = "module.bootstrap.coder_script.b", shape = "box"]
		"[root] null_resource.workspace (expand)" [label = "null_resource.workspace", shape = "box"]
		"[root] module.bootstrap.coder_script.a (expand)" -> "[root] module.bootstrap.var.agent_id (expand)"
		"[root] module.bootstrap.coder_script.b (expand)" -> "[root] module.bootstrap.var.agent_id (expand)"
		"[root] module.bootstrap.var.agent_id (expand)" -> "[root] coder_agent.main (expand)"
		"[root] null_resource.workspace (expand)" -> "[root] coder_agent.main (expand)"
	}`

	state, err := ConvertState(context.Background(), []*tfjson.StateModule{module}, graph, slogtest.Make(t, nil))
	require.NoError(t, err)
	require.Equal(t, &ScriptOrder{Graphs: []ScriptOrderGraph{
		scriptOrderTestGraph(
			"coder_agent.main",
			ScriptOrderPhaseStartup,
			"module.bootstrap.coder_script.a",
			"module.bootstrap.coder_script.b",
		),
	}}, state.ScriptOrder)
}

func TestConvertStateBuildsScriptOrderWithoutInfrastructureResource(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		{
			Address: "coder_agent.main",
			Mode:    tfjson.ManagedResourceMode,
			Type:    "coder_agent",
			Name:    "main",
			AttributeValues: map[string]any{
				"arch": "amd64",
			},
		},
		scriptOrderStateScript("coder_script.a", "a", "", true, false),
		scriptOrderStateScript("coder_script.b", "b", "", true, false),
		scriptOrderTestDataSource("data.coder_script_order.order", "order",
			scriptOrderTestRule{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}},
		),
	}}
	graph := `digraph {
		"[root] coder_agent.main (expand)" [label = "coder_agent.main", shape = "box"]
		"[root] coder_script.a (expand)" [label = "coder_script.a", shape = "box"]
		"[root] coder_script.b (expand)" [label = "coder_script.b", shape = "box"]
		"[root] coder_script.a (expand)" -> "[root] coder_agent.main (expand)"
		"[root] coder_script.b (expand)" -> "[root] coder_agent.main (expand)"
	}`

	state, err := ConvertState(context.Background(), []*tfjson.StateModule{module}, graph, slogtest.Make(t, nil))
	require.NoError(t, err)
	require.Empty(t, state.Resources)
	require.Equal(t, &ScriptOrder{Graphs: []ScriptOrderGraph{
		scriptOrderTestGraph("coder_agent.main", ScriptOrderPhaseStartup, "coder_script.a", "coder_script.b"),
	}}, state.ScriptOrder)
}

func scriptOrderTestDependency(script, prerequisite string, requirement ScriptOrderRequirement) ScriptOrderDependency {
	return ScriptOrderDependency{
		ScriptAddress:       script,
		PrerequisiteAddress: prerequisite,
		Requirement:         requirement,
	}
}

func scriptOrderTestGraph(runtimeAddress string, phase ScriptOrderPhase, prerequisite, script string) ScriptOrderGraph {
	return ScriptOrderGraph{
		RuntimeAddress: runtimeAddress,
		Phase:          phase,
		Scripts:        []string{prerequisite, script},
		Dependencies: []ScriptOrderDependency{
			scriptOrderTestDependency(script, prerequisite, ScriptOrderRequirementSuccess),
		},
	}
}

func scriptOrderStateScript(address, name, agentID string, runOnStart, runOnStop bool) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.ManagedResourceMode,
		Type:    "coder_script",
		Name:    name,
		AttributeValues: map[string]any{
			"agent_id":     agentID,
			"run_on_start": runOnStart,
			"run_on_stop":  runOnStop,
		},
	}
}
