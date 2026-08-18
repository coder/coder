package terraform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestScriptOrderEndToEnd(t *testing.T) {
	t.Parallel()

	for _, step := range []string{"plan", "state"} {
		t.Run(step, func(t *testing.T) {
			t.Parallel()

			modules, graph := loadScriptOrderFixture(t, "script-order", step)
			state, err := ConvertState(context.Background(), modules, graph, slogtest.Make(t, nil))
			require.NoError(t, err)

			require.Equal(t, &ScriptOrder{Graphs: []ScriptOrderGraph{
				scriptOrderTestGraph(
					"coder_agent.main",
					ScriptOrderPhaseShutdown,
					"coder_script.stop_a",
					"coder_script.stop_b",
				),
				{
					RuntimeAddress: "coder_agent.main",
					Phase:          ScriptOrderPhaseStartup,
					Scripts: []string{
						"coder_script.counted[0]",
						"coder_script.counted[1]",
						`coder_script.keyed["api"]`,
						`coder_script.keyed["worker"]`,
						"coder_script.root_start",
						"module.dependents.coder_script.module_script",
						"module.dependents.module.nested.coder_script.nested",
						"module.prerequisites.coder_script.module_script",
						"module.prerequisites.module.nested.coder_script.nested",
					},
					Dependencies: []ScriptOrderDependency{
						scriptOrderTestDependency("coder_script.counted[0]", "coder_script.root_start", ScriptOrderRequirementSuccess),
						scriptOrderTestDependency("coder_script.counted[1]", "coder_script.root_start", ScriptOrderRequirementSuccess),
						scriptOrderTestDependency(`coder_script.keyed["api"]`, "coder_script.counted[1]", ScriptOrderRequirementCompletion),
						scriptOrderTestDependency(`coder_script.keyed["worker"]`, "coder_script.counted[1]", ScriptOrderRequirementCompletion),
						scriptOrderTestDependency(`coder_script.keyed["worker"]`, "coder_script.root_start", ScriptOrderRequirementSuccess),
						scriptOrderTestDependency("module.dependents.coder_script.module_script", "module.prerequisites.coder_script.module_script", ScriptOrderRequirementCompletion),
						scriptOrderTestDependency("module.dependents.coder_script.module_script", "module.prerequisites.module.nested.coder_script.nested", ScriptOrderRequirementCompletion),
						scriptOrderTestDependency("module.dependents.module.nested.coder_script.nested", "module.dependents.coder_script.module_script", ScriptOrderRequirementSuccess),
						scriptOrderTestDependency("module.dependents.module.nested.coder_script.nested", "module.prerequisites.coder_script.module_script", ScriptOrderRequirementCompletion),
						scriptOrderTestDependency("module.dependents.module.nested.coder_script.nested", "module.prerequisites.module.nested.coder_script.nested", ScriptOrderRequirementCompletion),
						scriptOrderTestDependency("module.prerequisites.module.nested.coder_script.nested", "module.prerequisites.coder_script.module_script", ScriptOrderRequirementSuccess),
					},
				},
			}}, state.ScriptOrder)

			scripts := scriptOrderScriptsByAddress(state)
			require.Len(t, scripts, 12)
			unordered, ok := scripts["coder_script.unordered"]
			require.True(t, ok)
			require.Empty(t, unordered.Dependencies)

			worker, ok := scripts[`coder_script.keyed["worker"]`]
			require.True(t, ok)
			require.Equal(t, []*proto.ScriptDependency{
				{
					PrerequisiteResourceAddress: "coder_script.counted[1]",
					Requirement:                 proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_COMPLETION,
				},
				{
					PrerequisiteResourceAddress: "coder_script.root_start",
					Requirement:                 proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS,
				},
			}, worker.Dependencies)
		})
	}
}

func TestScriptOrderEndToEndRejectsCrossAgentDependency(t *testing.T) {
	t.Parallel()

	for _, step := range []string{"plan", "state"} {
		t.Run(step, func(t *testing.T) {
			t.Parallel()

			modules, graph := loadScriptOrderFixture(t, "script-order-cross-agent", step)
			state, err := ConvertState(context.Background(), modules, graph, slogtest.Make(t, nil))
			require.Error(t, err)
			require.Nil(t, state)
			require.ErrorContains(t, err, "data.coder_script_order.cross_agent")
			require.ErrorContains(t, err, "coder_agent.one")
			require.ErrorContains(t, err, "coder_agent.two")
			require.ErrorContains(t, err, "cross-agent dependencies are not supported")
		})
	}
}

func loadScriptOrderFixture(t *testing.T, name, step string) ([]*tfjson.StateModule, string) {
	t.Helper()

	directory := filepath.Join("testdata", "resources", name)
	raw, err := os.ReadFile(filepath.Join(directory, name+".tf"+step+".json"))
	require.NoError(t, err)

	var modules []*tfjson.StateModule
	switch step {
	case "plan":
		var plan tfjson.Plan
		require.NoError(t, json.Unmarshal(raw, &plan))
		modules = planModules(&plan)
	case "state":
		var state tfjson.State
		require.NoError(t, json.Unmarshal(raw, &state))
		modules = append(modules, state.Values.RootModule)
	default:
		t.Fatalf("unknown Terraform step %q", step)
	}

	graph, err := os.ReadFile(filepath.Join(directory, name+".tf"+step+".dot"))
	require.NoError(t, err)
	return modules, string(graph)
}

func scriptOrderScriptsByAddress(state *State) map[string]*proto.Script {
	scripts := map[string]*proto.Script{}
	for _, resource := range state.Resources {
		for _, agent := range resource.Agents {
			for _, script := range agent.Scripts {
				scripts[script.ResourceAddress] = script
			}
			for _, devcontainer := range agent.Devcontainers {
				for _, script := range devcontainer.Scripts {
					scripts[script.ResourceAddress] = script
				}
			}
		}
	}
	return scripts
}
