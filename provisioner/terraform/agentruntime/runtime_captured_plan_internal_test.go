package agentruntime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform/tfgraph"
)

func TestRuntimeResolverMatchesCapturedIndirectPlan(t *testing.T) {
	t.Parallel()

	fixtureDir := filepath.Join(
		"..", "testdata", "agent-runtime", "indirect-runtime",
	)
	planRaw, err := os.ReadFile(filepath.Join(
		fixtureDir, "indirect-runtime.tfplan.json",
	))
	require.NoError(t, err)
	var plan tfjson.Plan
	require.NoError(t, json.Unmarshal(planRaw, &plan))
	require.NotNil(t, plan.PlannedValues)
	require.NotNil(t, plan.PlannedValues.RootModule)
	require.NotNil(t, plan.Config)

	graphRaw, err := os.ReadFile(filepath.Join(
		fixtureDir, "indirect-runtime.tfplan.saved.dot",
	))
	require.NoError(t, err)
	graph, err := tfgraph.Parse(t.Context(), string(graphRaw))
	require.NoError(t, err)
	configIndex, err := newConfigIndex(t.Context(), plan.Config)
	require.NoError(t, err)
	require.NoError(t, configIndex.indexRuntimeSourceExpressions(
		t.Context(), fixtureDir,
	))

	resources := map[string]*tfjson.StateResource{}
	var agentAddresses []string
	var devcontainerAddresses []string
	var indexModule func(*tfjson.StateModule)
	indexModule = func(module *tfjson.StateModule) {
		if module == nil {
			return
		}
		for _, resource := range module.Resources {
			if resource == nil || resource.Mode != tfjson.ManagedResourceMode {
				continue
			}
			resources[resource.Address] = resource
			switch resource.Type {
			case "coder_agent":
				agentAddresses = append(agentAddresses, resource.Address)
			case "coder_devcontainer":
				devcontainerAddresses = append(
					devcontainerAddresses, resource.Address,
				)
			}
		}
		for _, child := range module.ChildModules {
			indexModule(child)
		}
	}
	indexModule(plan.PlannedValues.RootModule)

	tests := []struct {
		name     string
		address  string
		expected Target
	}{
		{
			name:    "LocalAgent",
			address: "coder_script.local_agent",
			expected: Target{
				Kind: KindWorkspaceAgent, Address: "coder_agent.root",
			},
		},
		{
			name:    "LocalDevcontainerParent",
			address: "coder_script.local_parent",
			expected: Target{
				Kind: KindWorkspaceAgent, Address: "coder_agent.root",
			},
		},
		{
			name:    "ForEachModuleOutputAgent",
			address: `coder_script.module_agent["api"]`,
			expected: Target{
				Kind:    KindWorkspaceAgent,
				Address: `module.runtime["api"].coder_agent.main`,
			},
		},
		{
			name:    "ForEachModuleOutputDevcontainerParent",
			address: `coder_script.module_parent["api"]`,
			expected: Target{
				Kind:    KindWorkspaceAgent,
				Address: `module.runtime["api"].coder_agent.main`,
			},
		},
		{
			name:    "RepeatedModuleInput",
			address: `module.feature["api"].coder_script.work`,
			expected: Target{
				Kind: KindWorkspaceAgent, Address: `coder_agent.service["api"]`,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resource := resources[test.address]
			require.NotNil(t, resource)
			resolver, err := NewResolver(
				t.Context(),
				graph,
				&Program{configIndex: configIndex},
				runtimeResolverTestTargets(
					agentAddresses, devcontainerAddresses,
				),
			)
			require.NoError(t, err)
			actual, err := resolver.ResolveResourceRuntime(
				t.Context(), resource,
			)
			require.NoError(t, err)
			require.Equal(t, test.expected, actual)
		})
	}
}
