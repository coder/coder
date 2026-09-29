package tfgraph_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform/tfgraph"
)

func TestQueryConfigurationNodesForReferences(t *testing.T) {
	t.Parallel()

	index, err := tfgraph.Parse(t.Context(), `digraph {
		"[root] module.开发.local.bridge (expand)"
		"[root] module.开发.local.bridge.attribute (expand)"
		"[root] module.runtime.output.agent_id (expand)"
	}`)
	require.NoError(t, err)

	for _, test := range []struct {
		name          string
		moduleAddress string
		references    []string
		expected      []string
	}{
		{
			name:          "DeclaringModuleAndMostSpecificAddress",
			moduleAddress: `module.开发["环境"]`,
			references:    []string{"local.bridge.attribute.value"},
			expected:      []string{"module.开发.local.bridge.attribute"},
		},
		{
			name:       "ModuleOutput",
			references: []string{`module.runtime["primary"].agent_id`},
			expected:   []string{"module.runtime.output.agent_id"},
		},
		{
			name:       "DuplicateReference",
			references: []string{"module.runtime.agent_id", "module.runtime.agent_id"},
			expected:   []string{"module.runtime.output.agent_id"},
		},
		{
			name:       "MissingReference",
			references: []string{"local.missing"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			query, err := tfgraph.NewQuery(index)
			require.NoError(t, err)
			nodes, err := query.ConfigurationNodesForReferences(
				t.Context(), test.moduleAddress, test.references,
			)
			require.NoError(t, err)
			if len(test.expected) == 0 {
				require.Empty(t, nodes)
				return
			}
			require.Equal(t, test.expected, nodeAddresses(t, index, nodes))
		})
	}
}

func TestQueryReachableTerminalNodes(t *testing.T) {
	t.Parallel()

	index, err := tfgraph.Parse(t.Context(), `digraph {
		"[root] coder_script.direct[\"api\"]"
		"[root] coder_agent.service[\"api\"]"
		"[root] module.service[\"worker\"].coder_script.setup"
		"[root] module.service[\"worker\"].coder_devcontainer.repo"
		"[root] module.service[\"worker\"].coder_agent.main"
		"[root] coder_script.direct[\"api\"]" -> "[root] coder_agent.service[\"api\"]"
		"[root] module.service[\"worker\"].coder_script.setup" -> "[root] module.service[\"worker\"].coder_devcontainer.repo"
		"[root] module.service[\"worker\"].coder_devcontainer.repo" -> "[root] module.service[\"worker\"].coder_agent.main"
	}`)
	require.NoError(t, err)

	resourceTerminal := func(node tfgraph.Node) bool {
		return strings.Contains(node.Address(), ".coder_agent.") ||
			strings.HasPrefix(node.Address(), "coder_agent.") ||
			strings.Contains(node.Address(), ".coder_devcontainer.") ||
			strings.HasPrefix(node.Address(), "coder_devcontainer.")
	}
	agentTerminal := func(node tfgraph.Node) bool {
		return strings.Contains(node.Address(), ".coder_agent.") ||
			strings.HasPrefix(node.Address(), "coder_agent.")
	}

	query, err := tfgraph.NewQuery(index)
	require.NoError(t, err)
	directStart := index.NodesForInstanceAddress(`coder_script.direct["api"]`)
	terminals, err := query.ReachableTerminalNodes(
		t.Context(), directStart, resourceTerminal,
	)
	require.NoError(t, err)
	require.Equal(
		t, []string{`coder_agent.service["api"]`},
		nodeAddresses(t, index, terminals),
	)

	repeatedModuleStart := index.NodesForInstanceAddress(
		`module.service["worker"].coder_script.setup`,
	)
	terminals, err = query.ReachableTerminalNodes(
		t.Context(), repeatedModuleStart, resourceTerminal,
	)
	require.NoError(t, err)
	require.Equal(
		t, []string{`module.service["worker"].coder_devcontainer.repo`},
		nodeAddresses(t, index, terminals),
	)

	// Resolving a devcontainer's parent deliberately traverses through the
	// devcontainer instead of treating it as a terminal runtime.
	terminals, err = query.ReachableTerminalNodes(
		t.Context(), repeatedModuleStart, agentTerminal,
	)
	require.NoError(t, err)
	require.Equal(
		t, []string{`module.service["worker"].coder_agent.main`},
		nodeAddresses(t, index, terminals),
	)
}

func TestQueryMatchesCapturedTerraformGraphs(t *testing.T) {
	t.Parallel()

	t.Run("SavedPlanRepeatedRuntime", func(t *testing.T) {
		t.Parallel()

		index := graphIndexFromFile(
			t, "testdata/repeated-runtime.tfplan.saved.dot",
		)
		start := index.NodesForInstanceAddress("coder_script.prerequisite")
		require.Len(t, start, 1)
		query, err := tfgraph.NewQuery(index)
		require.NoError(t, err)
		terminals, err := query.ReachableTerminalNodes(
			t.Context(), start,
			func(node tfgraph.Node) bool {
				return strings.HasPrefix(
					node.InstanceAddress(), "coder_devcontainer.repo[",
				)
			},
		)
		require.NoError(t, err)
		require.Equal(
			t, []string{"coder_devcontainer.repo[1]"},
			nodeAddresses(t, index, terminals),
		)
	})

	t.Run("Devcontainer", func(t *testing.T) {
		t.Parallel()

		index := graphIndexFromFile(
			t, "../testdata/resources/devcontainer/devcontainer.tfplan.dot",
		)
		start := index.NodesForConfigurationAddress("coder_devcontainer.dev1")
		require.Len(t, start, 1)
		query, err := tfgraph.NewQuery(index)
		require.NoError(t, err)
		terminals, err := query.ReachableTerminalNodes(
			t.Context(), start,
			func(node tfgraph.Node) bool {
				return node.ConfigurationAddress() == "coder_agent.main"
			},
		)
		require.NoError(t, err)
		require.Equal(
			t, []string{"coder_agent.main"},
			nodeAddresses(t, index, terminals),
		)
	})

	t.Run("ModuleInput", func(t *testing.T) {
		t.Parallel()

		index := graphIndexFromFile(
			t,
			"../testdata/resources/calling-module/calling-module.tfplan.dot",
		)
		query, err := tfgraph.NewQuery(index)
		require.NoError(t, err)
		start, err := query.ConfigurationNodesForReferences(
			t.Context(), "module.module", []string{"var.script"},
		)
		require.NoError(t, err)
		require.Equal(
			t, []string{"module.module.var.script"},
			nodeAddresses(t, index, start),
		)
		terminals, err := query.ReachableTerminalNodes(
			t.Context(), start,
			func(node tfgraph.Node) bool {
				return node.ConfigurationAddress() == "coder_agent.main"
			},
		)
		require.NoError(t, err)
		require.Equal(
			t, []string{"coder_agent.main"},
			nodeAddresses(t, index, terminals),
		)
	})
}

func TestQueryResultsAreDeterministic(t *testing.T) {
	t.Parallel()

	index, err := tfgraph.Parse(t.Context(), `digraph {
		"[root] local.start (expand)"
		"[root] null_resource.second"
		"[root] null_resource.first"
		"[root] local.start (expand)" -> "[root] null_resource.second"
		"[root] local.start (expand)" -> "[root] null_resource.first"
	}`)
	require.NoError(t, err)
	query, err := tfgraph.NewQuery(index)
	require.NoError(t, err)
	start := index.NodesForConfigurationAddress("local.start")
	require.Len(t, start, 1)
	start = append(start, start[0])

	for range 3 {
		terminals, err := query.ReachableTerminalNodes(
			t.Context(), start, func(node tfgraph.Node) bool {
				return strings.HasPrefix(node.Address(), "null_resource.")
			},
		)
		require.NoError(t, err)
		require.Equal(
			t,
			[]string{"null_resource.first", "null_resource.second"},
			nodeAddresses(t, index, terminals),
		)
	}
}

func TestQueryRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	_, err := tfgraph.NewQuery(nil)
	require.ErrorContains(t, err, "Terraform graph index is required")

	first, err := tfgraph.Parse(t.Context(), `digraph { "[root] local.first" }`)
	require.NoError(t, err)
	second, err := tfgraph.Parse(t.Context(), `digraph { "[root] local.second" }`)
	require.NoError(t, err)
	query, err := tfgraph.NewQuery(first)
	require.NoError(t, err)

	_, err = query.ReachableTerminalNodes(
		t.Context(), []tfgraph.NodeID{{}}, func(tfgraph.Node) bool { return false },
	)
	require.ErrorContains(t, err, "outside its index")
	_, err = query.ReachableTerminalNodes(
		t.Context(), second.NodesForInstanceAddress("local.second"),
		func(tfgraph.Node) bool { return false },
	)
	require.ErrorContains(t, err, "outside its index")
	_, err = query.ReachableTerminalNodes(t.Context(), nil, nil)
	require.ErrorContains(t, err, "terminal predicate is required")
}

func TestQueryHonorsCancellation(t *testing.T) {
	t.Parallel()

	index, err := tfgraph.Parse(t.Context(), `digraph {
		"[root] local.bridge (expand)"
	}`)
	require.NoError(t, err)
	query, err := tfgraph.NewQuery(index)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err = query.ConfigurationNodesForReferences(
		ctx, "", []string{"local.bridge"},
	)
	require.ErrorIs(t, err, context.Canceled)
	_, err = query.ReachableTerminalNodes(
		ctx, index.NodesForConfigurationAddress("local.bridge"),
		func(tfgraph.Node) bool { return false },
	)
	require.ErrorIs(t, err, context.Canceled)
}

func graphIndexFromFile(t *testing.T, path string) *tfgraph.Index {
	t.Helper()

	rawGraph, err := os.ReadFile(path)
	require.NoError(t, err)
	index, err := tfgraph.Parse(t.Context(), string(rawGraph))
	require.NoError(t, err)
	return index
}

func nodeAddresses(
	t *testing.T,
	index *tfgraph.Index,
	nodeIDs []tfgraph.NodeID,
) []string {
	t.Helper()

	addresses := make([]string, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		node, ok := index.Node(nodeID)
		require.True(t, ok)
		addresses = append(addresses, node.Address())
	}
	return addresses
}
