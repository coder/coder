package tfgraph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform/tfgraph"
)

const addressGraphDOT = `digraph {
	"[root] coder_script.direct (expand)"
	"[root] module.runtime.output.agent_id (expand)"
	"[root] coder_devcontainer.repo (expand)"
	"[root] coder_script.work[\"api\"]"
	"[root] coder_agent.main[\"} ->\"]"
	"[root] module.service[\"worker\"].coder_agent.main"
	"[root] data.coder_script_order.old (destroy)"
}`

func TestIndexAddresses(t *testing.T) {
	t.Parallel()

	index, err := tfgraph.Parse(t.Context(), addressGraphDOT)
	require.NoError(t, err)

	for _, address := range []string{
		"coder_script.direct",
		"module.runtime.output.agent_id",
		"coder_devcontainer.repo",
	} {
		t.Run("Configuration/"+address, func(t *testing.T) {
			t.Parallel()

			node := requireSingleNode(
				t, index, index.NodesForConfigurationAddress(address),
			)
			require.Equal(t, address, node.Address())
			require.Equal(t, address, node.ConfigurationAddress())
			require.Empty(t, node.InstanceAddress())
			require.Equal(t, "expand", node.Operation())
		})
	}
	for _, address := range []string{
		`coder_script.work["api"]`,
		`coder_agent.main["} ->"]`,
		`module.service["worker"].coder_agent.main`,
	} {
		t.Run("Instance/"+address, func(t *testing.T) {
			t.Parallel()

			node := requireSingleNode(
				t, index, index.NodesForInstanceAddress(address),
			)
			require.Equal(t, address, node.Address())
			require.Empty(t, node.ConfigurationAddress())
			require.Equal(t, address, node.InstanceAddress())
			require.Empty(t, node.Operation())
		})
	}
	t.Run("Missing", func(t *testing.T) {
		t.Parallel()

		require.Empty(t, index.NodesForConfigurationAddress("coder_script.missing"))
		require.Empty(t, index.NodesForInstanceAddress("coder_script.missing"))
	})
	// Saved-plan graphs retain destroy nodes for data-resource deletes omitted
	// from plan JSON.
	t.Run("Destroy", func(t *testing.T) {
		t.Parallel()

		var destroyNode *tfgraph.Node
		for _, node := range graphNodes(index) {
			if node.Operation() == "destroy" {
				destroyNode = &node
				break
			}
		}
		require.NotNil(t, destroyNode)
		require.Equal(t, "data.coder_script_order.old", destroyNode.Address())
		require.Empty(t, destroyNode.ConfigurationAddress())
		require.Empty(t, destroyNode.InstanceAddress())
	})
}

func TestIndexNodeOrderIsDeterministic(t *testing.T) {
	t.Parallel()

	firstIndex, err := tfgraph.Parse(t.Context(), addressGraphDOT)
	require.NoError(t, err)
	secondIndex, err := tfgraph.Parse(t.Context(), addressGraphDOT)
	require.NoError(t, err)
	require.Equal(t,
		graphNodeDescriptors(firstIndex),
		graphNodeDescriptors(secondIndex),
	)
}

func TestIndexAddressLookupsReturnCopies(t *testing.T) {
	t.Parallel()

	index, err := tfgraph.Parse(t.Context(), addressGraphDOT)
	require.NoError(t, err)

	for _, test := range []struct {
		name   string
		lookup func() []tfgraph.NodeID
	}{
		{
			name: "Configuration",
			lookup: func() []tfgraph.NodeID {
				return index.NodesForConfigurationAddress("coder_script.direct")
			},
		},
		{
			name: "Instance",
			lookup: func() []tfgraph.NodeID {
				return index.NodesForInstanceAddress(`coder_script.work["api"]`)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			nodes := test.lookup()
			require.Len(t, nodes, 1)
			original := nodes[0]
			nodes[0] = tfgraph.NodeID{}
			require.Equal(t, original, test.lookup()[0])
		})
	}
}

func TestNodeIDIsScopedToIndex(t *testing.T) {
	t.Parallel()

	firstIndex, err := tfgraph.Parse(t.Context(), addressGraphDOT)
	require.NoError(t, err)
	secondIndex, err := tfgraph.Parse(t.Context(), addressGraphDOT)
	require.NoError(t, err)

	ids := firstIndex.NodesForConfigurationAddress("coder_script.direct")
	require.Len(t, ids, 1)
	node, ok := firstIndex.Node(ids[0])
	require.True(t, ok)
	require.Equal(t, `"[root] coder_script.direct (expand)"`, node.RawID())

	_, ok = secondIndex.Node(ids[0])
	require.False(t, ok)
	_, ok = firstIndex.Node(tfgraph.NodeID{})
	require.False(t, ok)
}

func TestParseCapturedTerraformGraphs(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name                   string
		path                   string
		configurationAddresses []string
		instanceAddresses      []string
	}{
		// The captured saved-plan apply graph contains a count-expanded
		// devcontainer and scripts whose agent IDs reference one concrete
		// devcontainer instance.
		{
			name: "SavedPlanRepeatedRuntime",
			path: "testdata/repeated-runtime.tfplan.saved.dot",
			configurationAddresses: []string{
				"coder_devcontainer.repo",
				"coder_script.prerequisite",
			},
			instanceAddresses: []string{
				"coder_devcontainer.repo[0]",
				"coder_devcontainer.repo[1]",
				"coder_script.prerequisite",
			},
		},
		// The captured config-derived plan graph contains root-module
		// devcontainers that reference a root-module agent.
		{
			name: "Devcontainer",
			path: "../testdata/resources/devcontainer/devcontainer.tfplan.dot",
			configurationAddresses: []string{
				"coder_agent.main",
				"coder_devcontainer.dev1",
			},
		},
		// The captured config-derived plan graph passes a root-module agent
		// attribute into a child-module input variable.
		{
			name: "ModuleInput",
			path: "../testdata/resources/calling-module/calling-module.tfplan.dot",
			configurationAddresses: []string{
				"coder_agent.main",
				"module.module.var.script",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			index := graphIndexFromFile(t, test.path)
			for _, address := range test.configurationAddresses {
				require.NotEmpty(
					t, index.NodesForConfigurationAddress(address), address,
				)
			}
			for _, address := range test.instanceAddresses {
				require.NotEmpty(
					t, index.NodesForInstanceAddress(address), address,
				)
			}
		})
	}
}

type graphNodeDescriptor struct {
	rawID                string
	address              string
	configurationAddress string
	instanceAddress      string
	operation            string
}

func graphNodeDescriptors(index *tfgraph.Index) []graphNodeDescriptor {
	descriptors := make([]graphNodeDescriptor, 0)
	for _, node := range graphNodes(index) {
		descriptors = append(descriptors, graphNodeDescriptor{
			rawID:                node.RawID(),
			address:              node.Address(),
			configurationAddress: node.ConfigurationAddress(),
			instanceAddress:      node.InstanceAddress(),
			operation:            node.Operation(),
		})
	}
	return descriptors
}

func graphNodes(index *tfgraph.Index) []tfgraph.Node {
	nodes := make([]tfgraph.Node, 0)
	for _, node := range index.Nodes() {
		nodes = append(nodes, node)
	}
	return nodes
}

func requireSingleNode(
	t *testing.T,
	index *tfgraph.Index,
	nodeIDs []tfgraph.NodeID,
) tfgraph.Node {
	t.Helper()
	require.Len(t, nodeIDs, 1)
	node, ok := index.Node(nodeIDs[0])
	require.True(t, ok)
	return node
}
