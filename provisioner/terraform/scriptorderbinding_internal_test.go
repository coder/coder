package terraform

import (
	"os"
	"path/filepath"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/provisioner/terraform/agentruntime"
	"github.com/coder/coder/v2/provisioner/terraform/scriptorder"
	"github.com/coder/coder/v2/provisioner/terraform/tfgraph"
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
		&scriptOrderRuntimeBindingInput{
			source: scriptOrderConversionSourceState, program: program,
		},
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

func TestConvertStateBindsPostApplyScriptRuntimes(t *testing.T) {
	t.Parallel()

	module := scriptOrderRuntimeBindingTestModule(
		scriptOrderRuntimeBindingTestAgent(
			"coder_agent.main", "main", "agent-id",
		),
		scriptOrderRuntimeBindingTestDevcontainer(
			"coder_devcontainer.repo", "repo", "agent-id", "subagent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.work", "work", "agent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.prepare", "prepare", "subagent-id",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.unselected", "unselected", "missing-id",
		),
	)
	conversion, err := convertState(
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

	agents := scriptOrderRuntimeBindingTestAgents(conversion.state)
	require.Len(t, agents, 1)
	require.Len(t, agents[0].Scripts, 1)
	require.Equal(t, "work", agents[0].Scripts[0].DisplayName)
	require.Len(t, agents[0].Devcontainers, 1)
	require.Len(t, agents[0].Devcontainers[0].Scripts, 1)
	require.Equal(
		t, "prepare", agents[0].Devcontainers[0].Scripts[0].DisplayName,
	)

	require.Equal(
		t,
		"coder_agent.main",
		conversion.scriptOrder.scripts["coder_script.work"].RuntimeAddress,
	)
	require.Equal(
		t,
		"coder_devcontainer.repo",
		conversion.scriptOrder.scripts["coder_script.prepare"].RuntimeAddress,
	)
	require.Empty(
		t,
		conversion.scriptOrder.scripts["coder_script.unselected"].RuntimeAddress,
	)
	require.Empty(
		t,
		conversion.scriptOrder.scripts["coder_script.unselected"].RuntimeError,
	)
}

func TestConvertStateRecordsPostApplyRuntimeErrors(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		agents        []*tfjson.StateResource
		graph         string
		runtimeID     string
		errorContains string
	}{
		{
			name: "Missing",
			agents: []*tfjson.StateResource{
				scriptOrderRuntimeBindingTestAgent(
					"coder_agent.main", "main", "agent-id",
				),
			},
			graph:         scriptOrderRuntimeBindingTestConversionGraph(),
			runtimeID:     "missing-id",
			errorContains: "does not match any workspace agent or devcontainer subagent",
		},
		{
			name: "Ambiguous",
			agents: []*tfjson.StateResource{
				scriptOrderRuntimeBindingTestAgent(
					"coder_agent.first", "first", "duplicate-id",
				),
				scriptOrderRuntimeBindingTestAgent(
					"coder_agent.second", "second", "duplicate-id",
				),
				{
					Address: "docker_container.secondary",
					Mode:    tfjson.ManagedResourceMode,
					Type:    "docker_container",
					Name:    "secondary",
				},
			},
			graph: `digraph {
				"[root] docker_container.workspace" [label = "docker_container.workspace"]
				"[root] docker_container.secondary" [label = "docker_container.secondary"]
				"[root] coder_agent.first (expand)" [label = "coder_agent.first"]
				"[root] coder_agent.second (expand)" [label = "coder_agent.second"]
				"[root] docker_container.workspace" -> "[root] coder_agent.first (expand)"
				"[root] docker_container.secondary" -> "[root] coder_agent.second (expand)"
			}`,
			runtimeID:     "duplicate-id",
			errorContains: "matches multiple agent runtimes",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resources := append(
				test.agents,
				scriptOrderRuntimeBindingTestScript(
					"coder_script.work", "work", test.runtimeID,
				),
				scriptOrderRuntimeBindingTestScript(
					"coder_script.prepare", "prepare", test.runtimeID,
				),
			)
			module := scriptOrderRuntimeBindingTestModule(resources...)
			conversion, err := convertState(
				t.Context(),
				[]*tfjson.StateModule{module},
				test.graph,
				slogtest.Make(t, nil),
				&scriptOrderRuntimeBindingInput{
					source:  scriptOrderConversionSourceState,
					program: scriptOrderRuntimeBindingTestProgram(t, module),
				},
			)
			require.NoError(t, err)
			for _, address := range []string{
				"coder_script.work", "coder_script.prepare",
			} {
				require.Contains(
					t,
					conversion.scriptOrder.scripts[address].RuntimeError,
					test.errorContains,
				)
			}
			_, err = conversion.scriptOrder.prepared.Finalize(
				conversion.scriptOrder.scripts,
			)
			require.ErrorContains(t, err, test.errorContains)
		})
	}
}

func TestConvertStateResolvesPlannedScriptRuntimes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name                 string
		devcontainer         bool
		scriptReference      string
		expectedRuntime      string
		expectedDevcontainer bool
	}{
		{
			name:            "WorkspaceAgent",
			scriptReference: "coder_agent.main.id",
			expectedRuntime: "coder_agent.main",
		},
		{
			name:                 "DevcontainerSubagent",
			devcontainer:         true,
			scriptReference:      "coder_devcontainer.repo.subagent_id",
			expectedRuntime:      "coder_devcontainer.repo",
			expectedDevcontainer: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resources := []*tfjson.StateResource{
				scriptOrderRuntimeBindingTestAgent(
					"coder_agent.main", "main", "",
				),
				scriptOrderRuntimeBindingTestScript(
					"coder_script.work", "work", "",
				),
				scriptOrderRuntimeBindingTestScript(
					"coder_script.prepare", "prepare", "",
				),
			}
			configResources := []*tfjson.ConfigResource{
				scriptOrderRuntimeBindingTestConfigResource(
					"coder_script", "work", test.scriptReference,
				),
				scriptOrderRuntimeBindingTestConfigResource(
					"coder_script", "prepare", test.scriptReference,
				),
			}
			if test.devcontainer {
				resources = append(resources,
					scriptOrderRuntimeBindingTestDevcontainer(
						"coder_devcontainer.repo", "repo", "", "",
					),
				)
				configResources = append(configResources,
					scriptOrderRuntimeBindingTestConfigResource(
						"coder_devcontainer", "repo", "coder_agent.main.id",
					),
				)
			}
			module := scriptOrderRuntimeBindingTestModule(resources...)
			rawGraph := `digraph {
				"[root] docker_container.workspace" [label = "docker_container.workspace"]
				"[root] coder_agent.main (expand)" [label = "coder_agent.main"]
				"[root] coder_devcontainer.repo (expand)" [label = "coder_devcontainer.repo"]
				"[root] docker_container.workspace" -> "[root] coder_agent.main (expand)"
			}`
			planGraph, err := tfgraph.Parse(t.Context(), rawGraph)
			require.NoError(t, err)
			conversion, err := convertState(
				t.Context(),
				[]*tfjson.StateModule{module},
				rawGraph,
				slogtest.Make(t, nil),
				&scriptOrderRuntimeBindingInput{
					source: scriptOrderConversionSourcePlan,
					program: scriptOrderRuntimeBindingTestProgramWithConfig(
						t, module, configResources...,
					),
					runtimeProgram: scriptOrderRuntimeBindingTestAgentRuntimeProgram(
						t, configResources...,
					),
					planGraph: planGraph,
				},
			)
			require.NoError(t, err)
			require.Equal(
				t,
				test.expectedRuntime,
				conversion.scriptOrder.scripts["coder_script.work"].RuntimeAddress,
			)

			agents := scriptOrderRuntimeBindingTestAgents(conversion.state)
			require.Len(t, agents, 1)
			if test.expectedDevcontainer {
				require.Empty(t, agents[0].Scripts)
				require.Len(t, agents[0].Devcontainers, 1)
				require.Len(t, agents[0].Devcontainers[0].Scripts, 2)
				return
			}
			require.Len(t, agents[0].Scripts, 2)
		})
	}
}

func TestConvertStateResolvesIndirectPlannedScriptRuntime(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(workdir, "locals.tf"),
		[]byte(`locals {
  parent_agent_id = coder_devcontainer.repo.agent_id
}

resource "coder_agent" "main" {}

resource "coder_devcontainer" "repo" {
  agent_id = coder_agent.main.id
}

resource "coder_script" "work" {
  agent_id = local.parent_agent_id
}

resource "coder_script" "prepare" {
  agent_id = local.parent_agent_id
}
`),
		0o600,
	))
	module := scriptOrderRuntimeBindingTestModule(
		scriptOrderRuntimeBindingTestAgent(
			"coder_agent.main", "main", "",
		),
		scriptOrderRuntimeBindingTestDevcontainer(
			"coder_devcontainer.repo", "repo", "", "",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.work", "work", "",
		),
		scriptOrderRuntimeBindingTestScript(
			"coder_script.prepare", "prepare", "",
		),
	)
	configResources := []*tfjson.ConfigResource{
		scriptOrderRuntimeBindingTestConfigResource(
			"coder_devcontainer", "repo", "coder_agent.main.id",
		),
		scriptOrderRuntimeBindingTestConfigResource(
			"coder_script", "work", "local.parent_agent_id",
		),
		scriptOrderRuntimeBindingTestConfigResource(
			"coder_script", "prepare", "local.parent_agent_id",
		),
	}
	program := scriptOrderRuntimeBindingTestProgramWithConfig(
		t, module, configResources...,
	)
	runtimeProgram := scriptOrderRuntimeBindingTestAgentRuntimeProgram(
		t, configResources...,
	)
	require.NoError(t, runtimeProgram.LoadProvenance(t.Context(), workdir))
	rawGraph := `digraph {
		"[root] docker_container.workspace" [label = "docker_container.workspace"]
		"[root] coder_agent.main (expand)" [label = "coder_agent.main"]
		"[root] coder_devcontainer.repo (expand)" [label = "coder_devcontainer.repo"]
		"[root] docker_container.workspace" -> "[root] coder_agent.main (expand)"
	}`
	planGraph, err := tfgraph.Parse(t.Context(), rawGraph)
	require.NoError(t, err)
	conversion, err := convertState(
		t.Context(),
		[]*tfjson.StateModule{module},
		rawGraph,
		slogtest.Make(t, nil),
		&scriptOrderRuntimeBindingInput{
			source:         scriptOrderConversionSourcePlan,
			program:        program,
			runtimeProgram: runtimeProgram,
			planGraph:      planGraph,
		},
	)
	require.NoError(t, err)
	require.Empty(
		t,
		conversion.scriptOrder.scripts["coder_script.work"].RuntimeError,
	)
	require.Equal(
		t,
		"coder_agent.main",
		conversion.scriptOrder.scripts["coder_script.work"].RuntimeAddress,
	)
	agents := scriptOrderRuntimeBindingTestAgents(conversion.state)
	require.Len(t, agents, 1)
	require.Len(t, agents[0].Scripts, 2)
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
			"display_name": name,
			"run_on_start": true,
		},
	}
}

func scriptOrderRuntimeBindingTestProgram(
	t *testing.T,
	module *tfjson.StateModule,
) *scriptorder.Program {
	t.Helper()

	var configResources []*tfjson.ConfigResource
	for _, resource := range module.Resources {
		if resource.Mode != tfjson.ManagedResourceMode ||
			resource.Type != "coder_script" {
			continue
		}
		configResources = append(configResources, &tfjson.ConfigResource{
			Address: resource.Address,
			Mode:    resource.Mode,
			Type:    resource.Type,
			Name:    resource.Name,
		})
	}
	return scriptOrderRuntimeBindingTestProgramWithConfig(
		t, module, configResources...,
	)
}

func scriptOrderRuntimeBindingTestProgramWithConfig(
	t *testing.T,
	module *tfjson.StateModule,
	configResources ...*tfjson.ConfigResource,
) *scriptorder.Program {
	t.Helper()

	config := scriptOrderRuntimeBindingTestConfig(configResources...)
	program, err := scriptorder.NewProgram(
		t.Context(), []*tfjson.StateModule{module}, config,
	)
	require.NoError(t, err)
	require.NotNil(t, program)
	return program
}

func scriptOrderRuntimeBindingTestAgentRuntimeProgram(
	t *testing.T,
	configResources ...*tfjson.ConfigResource,
) *agentruntime.Program {
	t.Helper()

	program, err := agentruntime.NewProgram(
		t.Context(), scriptOrderRuntimeBindingTestConfig(configResources...),
	)
	require.NoError(t, err)
	require.NotNil(t, program)
	return program
}

func scriptOrderRuntimeBindingTestConfig(
	configResources ...*tfjson.ConfigResource,
) *tfjson.Config {
	resources := []*tfjson.ConfigResource{{
		Address: "data.coder_script_order.order",
		Mode:    tfjson.DataResourceMode,
		Type:    "coder_script_order",
		Name:    "order",
	}}
	resources = append(resources, configResources...)
	return &tfjson.Config{RootModule: &tfjson.ConfigModule{
		Resources: resources,
	}}
}

func scriptOrderRuntimeBindingTestConfigResource(
	resourceType string,
	name string,
	references ...string,
) *tfjson.ConfigResource {
	return &tfjson.ConfigResource{
		Address: resourceType + "." + name,
		Mode:    tfjson.ManagedResourceMode,
		Type:    resourceType,
		Name:    name,
		Expressions: map[string]*tfjson.Expression{
			"agent_id": {
				ExpressionData: &tfjson.ExpressionData{
					References: references,
				},
			},
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
