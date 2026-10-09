package terraform

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
	protobuf "google.golang.org/protobuf/proto"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/provisioner/terraform/scriptorder"
	"github.com/coder/coder/v2/provisioner/terraform/tfgraph"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestConvertStateWithScriptOrderFinalizesOrder(t *testing.T) {
	t.Parallel()

	module := scriptOrderRuntimeBindingTestModule(
		scriptOrderRuntimeBindingTestAgent(
			"coder_agent.main", "main", "agent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.start_work", "start_work", "agent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.start_prepare", "start_prepare", "agent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.stop_work", "stop_work", "agent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.stop_prepare", "stop_prepare", "agent-id",
		),
	)
	for _, resource := range module.Resources {
		if resource.Address == "coder_script.stop_work" ||
			resource.Address == "coder_script.stop_prepare" {
			resource.AttributeValues["run_on_start"] = false
			resource.AttributeValues["run_on_stop"] = true
		}
		if resource.Address == "data.coder_script_order.order" {
			resource.AttributeValues["rule"] = []any{
				map[string]any{
					"run":   []string{"coder_script.start_work"},
					"after": []string{"coder_script.start_prepare"},
					"phase": "start",
				},
				map[string]any{
					"run":   []string{"coder_script.stop_work"},
					"after": []string{"coder_script.stop_prepare"},
					"phase": "stop",
				},
			}
		}
	}
	conversion, err := convertStateWithScriptOrder(
		t.Context(),
		[]*tfjson.StateModule{module},
		scriptOrderRuntimeBindingTestConversionGraph(),
		slogtest.Make(t, nil),
		&scriptOrderRuntimeBindingInput{
			source:  scriptOrderConversionSourceState,
			program: scriptOrderRuntimeBindingTestProgram(t, module),
		},
	)
	require.NoError(t, err)
	require.Equal(t, &scriptorder.ScriptOrder{
		Graphs: []scriptorder.Graph{
			{
				RuntimeAddress: "coder_agent.main",
				Phase:          scriptorder.ScriptOrderPhaseStart,
				Dependencies: []scriptorder.Dependency{{
					DependentAddress:    "coder_script.start_work",
					PrerequisiteAddress: "coder_script.start_prepare",
					Requirement:         scriptorder.ScriptOrderRequirementSuccess,
				}},
			},
			{
				RuntimeAddress: "coder_agent.main",
				Phase:          scriptorder.ScriptOrderPhaseStop,
				Dependencies: []scriptorder.Dependency{{
					DependentAddress:    "coder_script.stop_work",
					PrerequisiteAddress: "coder_script.stop_prepare",
					Requirement:         scriptorder.ScriptOrderRequirementSuccess,
				}},
			},
		},
	}, conversion.order)
}

func TestLogScriptOrderWarnings(t *testing.T) {
	t.Parallel()

	sink := &mockLogger{}
	logScriptOrderWarnings(sink, []string{"first warning", "second warning"}, 2)
	require.Equal(t, []*proto.Log{
		{Level: proto.LogLevel_WARN, Output: "first warning"},
		{Level: proto.LogLevel_WARN, Output: "second warning"},
		{
			Level: proto.LogLevel_WARN,
			Output: fmt.Sprintf(
				"2 additional script order warnings were omitted; at most %d are reported per Terraform conversion",
				scriptorder.MaxWarnings,
			),
		},
	}, sink.logs)
}

func TestConvertStateWithScriptOrderFinalizationError(t *testing.T) {
	t.Parallel()

	module := scriptOrderRuntimeBindingTestModule(
		scriptOrderRuntimeBindingTestAgent(
			"coder_agent.main", "main", "main-agent-id",
		),
		scriptOrderRuntimeBindingTestAgent(
			"coder_agent.secondary", "secondary", "secondary-agent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.work", "work", "main-agent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.prepare", "prepare", "secondary-agent-id",
		),
	)
	_, err := convertStateWithScriptOrder(
		t.Context(),
		[]*tfjson.StateModule{module},
		`digraph {
			"[root] docker_container.workspace" [label = "docker_container.workspace"]
			"[root] coder_agent.main (expand)" [label = "coder_agent.main"]
			"[root] coder_agent.secondary (expand)" [label = "coder_agent.secondary"]
			"[root] docker_container.workspace" -> "[root] coder_agent.main (expand)"
			"[root] docker_container.workspace" -> "[root] coder_agent.secondary (expand)"
		}`,
		slogtest.Make(t, nil),
		&scriptOrderRuntimeBindingInput{
			source:  scriptOrderConversionSourceState,
			program: scriptOrderRuntimeBindingTestProgram(t, module),
		},
	)
	require.ErrorContains(t, err, "resolve script order")
	require.ErrorContains(
		t, err,
		"scripts can be ordered only within the same agent or devcontainer subagent",
	)
}

func TestPrepareScriptOrderRuntimeBindingInputCreatesRuntimeProgramForPlanOnly(t *testing.T) {
	t.Parallel()

	module := scriptOrderRuntimeBindingTestModule(
		scriptOrderRuntimeBindingTestScript(
			"coder_script.work", "work", "agent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.prepare", "prepare", "agent-id",
		),
	)
	config := scriptOrderRuntimeBindingTestConfig(
		scriptOrderRuntimeBindingTestConfigResource(
			"coder_script", "work",
		),
		scriptOrderRuntimeBindingTestConfigResource(
			"coder_script", "prepare",
		),
	)

	for _, test := range []struct {
		name               string
		source             scriptOrderConversionSource
		wantRuntimeProgram bool
	}{
		{
			name:               "Plan",
			source:             scriptOrderConversionSourcePlan,
			wantRuntimeProgram: true,
		},
		{
			name:   "State",
			source: scriptOrderConversionSourceState,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input, err := prepareScriptOrderRuntimeBindingInput(
				t.Context(), []*tfjson.StateModule{module}, config, test.source,
			)
			require.NoError(t, err)
			require.NotNil(t, input)
			if test.wantRuntimeProgram {
				require.NotNil(t, input.runtimeProgram)
			} else {
				require.Nil(t, input.runtimeProgram)
			}
		})
	}
}

func TestFilterRemovedScriptOrderDataSources(t *testing.T) {
	t.Parallel()

	addresses := []string{
		"data.coder_script_order.removed",
		"data.coder_script_order.replaced",
		"data.coder_script_order.retained",
	}
	module := &tfjson.StateModule{}
	config := &tfjson.Config{RootModule: &tfjson.ConfigModule{}}
	for _, address := range addresses {
		name := address[len("data.coder_script_order."):]
		module.Resources = append(module.Resources, &tfjson.StateResource{
			Address: address,
			Mode:    tfjson.DataResourceMode,
			Type:    "coder_script_order",
			Name:    name,
		})
		config.RootModule.Resources = append(
			config.RootModule.Resources,
			&tfjson.ConfigResource{
				Address: address,
				Mode:    tfjson.DataResourceMode,
				Type:    "coder_script_order",
				Name:    name,
			},
		)
	}
	program, err := scriptorder.NewProgram(
		t.Context(), []*tfjson.StateModule{module}, config,
	)
	require.NoError(t, err)
	require.NotNil(t, program)
	graph, err := tfgraph.Parse(t.Context(), `digraph {
		"[root] data.coder_script_order.removed (destroy)"
		"[root] data.coder_script_order.replaced (destroy)"
		"[root] data.coder_script_order.replaced"
		"[root] data.coder_script_order.retained"
	}`)
	require.NoError(t, err)

	filtered, err := filterRemovedScriptOrderDataSources(
		t.Context(), graph, program,
	)
	require.NoError(t, err)
	require.Equal(t, []string{
		"data.coder_script_order.replaced",
		"data.coder_script_order.retained",
	}, filtered.DataSourceAddresses())
	require.Equal(t, addresses, program.DataSourceAddresses())

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = filterRemovedScriptOrderDataSources(ctx, graph, program)
	require.ErrorIs(t, err, context.Canceled)
}

func TestConvertStateWithScriptOrderAttachesDependencies(t *testing.T) {
	t.Parallel()

	module := scriptOrderRuntimeBindingTestModule(
		scriptOrderRuntimeBindingTestAgent(
			"coder_agent.main", "main", "agent-id",
		),
		scriptOrderRuntimeBindingTestDevcontainer(
			"coder_devcontainer.repo", "repo", "agent-id", "subagent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.clone_repo", "clone_repo", "agent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.install_tools", "install_tools", "agent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.build", "build", "agent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.dc_setup", "dc_setup", "subagent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.dc_run", "dc_run", "subagent-id",
		),
	)
	for _, resource := range module.Resources {
		if resource.Address == "data.coder_script_order.order" {
			resource.AttributeValues["rule"] = []any{
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
			}
		}
	}
	conversion, err := convertStateWithScriptOrder(
		t.Context(),
		[]*tfjson.StateModule{module},
		scriptOrderRuntimeBindingTestConversionGraph(),
		slogtest.Make(t, nil),
		&scriptOrderRuntimeBindingInput{
			source:  scriptOrderConversionSourceState,
			program: scriptOrderRuntimeBindingTestProgram(t, module),
		},
	)
	require.NoError(t, err)
	require.NotNil(t, conversion.order)

	agents := scriptOrderRuntimeBindingTestAgents(conversion.state)
	require.Len(t, agents, 1)
	require.Len(t, agents[0].Devcontainers, 1)
	agentScripts := scriptOrderConvertTestScriptsByAddress(t, agents[0].Scripts,
		"coder_script.build", "coder_script.clone_repo", "coder_script.install_tools",
	)
	devcontainerScripts := scriptOrderConvertTestScriptsByAddress(t, agents[0].Devcontainers[0].Scripts,
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

// scriptOrderConvertTestScriptsByAddress checks that scripts carry exactly
// the given resource addresses and returns them keyed by address.
func scriptOrderConvertTestScriptsByAddress(
	t *testing.T,
	scripts []*proto.Script,
	addresses ...string,
) map[string]*proto.Script {
	t.Helper()

	byAddress := make(map[string]*proto.Script, len(scripts))
	var got []string
	for _, script := range scripts {
		byAddress[script.ResourceAddress] = script
		got = append(got, script.ResourceAddress)
	}
	require.ElementsMatch(t, addresses, got)
	return byAddress
}

func TestCheckGraphCompleteSize(t *testing.T) {
	t.Parallel()

	script := &proto.Script{ResourceAddress: "coder_script.build"}
	response := &proto.GraphComplete{
		Resources: []*proto.Resource{{
			Agents: []*proto.Agent{{Scripts: []*proto.Script{script}}},
		}},
	}
	require.NoError(t, checkGraphCompleteSize(response))

	dependency := &proto.ScriptDependency{
		PrerequisiteResourceAddress: "module.setup." + strings.Repeat("x", 90) + ".coder_script.step",
		Requirement:                 proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS,
	}
	count := drpcsdk.MaxMessageSize/protobuf.Size(dependency) + 1
	for range count {
		script.Dependencies = append(script.Dependencies, dependency)
	}
	require.Greater(t, protobuf.Size(response), drpcsdk.MaxMessageSize)

	err := checkGraphCompleteSize(response)
	require.ErrorContains(t, err, strconv.Itoa(protobuf.Size(response))+" bytes")
	require.ErrorContains(t, err, strconv.Itoa(drpcsdk.MaxMessageSize)+" byte message limit")
	require.ErrorContains(t, err, "coder_script_order")
}
