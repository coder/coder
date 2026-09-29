package tfgraph_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform/tfgraph"
)

func TestIndexAddresses(t *testing.T) {
	t.Parallel()

	rawGraph := `digraph {
		"[root] coder_script.direct (expand)"
		"[root] module.runtime.output.agent_id (expand)"
		"[root] coder_devcontainer.repo (expand)"
		"[root] coder_script.work[\"api\"]"
		"[root] coder_agent.main[\"} ->\"]"
		"[root] module.service[\"worker\"].coder_agent.main"
		"[root] coder_agent.old (destroy)"
	}`
	first, err := tfgraph.Parse(t.Context(), rawGraph)
	require.NoError(t, err)
	second, err := tfgraph.Parse(t.Context(), rawGraph)
	require.NoError(t, err)
	require.Equal(t, graphNodeDescriptors(first), graphNodeDescriptors(second))

	for _, address := range []string{
		"coder_script.direct",
		"module.runtime.output.agent_id",
		"coder_devcontainer.repo",
	} {
		node := requireSingleNode(
			t, first, first.NodesForConfigurationAddress(address),
		)
		require.Equal(t, address, node.Address())
		require.Equal(t, address, node.ConfigurationAddress())
		require.Empty(t, node.InstanceAddress())
		require.Equal(t, "expand", node.Operation())
	}
	for _, address := range []string{
		`coder_script.work["api"]`,
		`coder_agent.main["} ->"]`,
		`module.service["worker"].coder_agent.main`,
	} {
		node := requireSingleNode(
			t, first, first.NodesForInstanceAddress(address),
		)
		require.Equal(t, address, node.Address())
		require.Empty(t, node.ConfigurationAddress())
		require.Equal(t, address, node.InstanceAddress())
		require.Empty(t, node.Operation())
	}
	require.Empty(t, first.NodesForConfigurationAddress("coder_script.missing"))
	require.Empty(t, first.NodesForInstanceAddress("coder_script.missing"))

	var destroyNode *tfgraph.Node
	for _, node := range graphNodes(first) {
		if node.Operation() == "destroy" {
			destroyNode = &node
			break
		}
	}
	require.NotNil(t, destroyNode)
	require.Equal(t, "coder_agent.old", destroyNode.Address())
	require.Empty(t, destroyNode.ConfigurationAddress())
	require.Empty(t, destroyNode.InstanceAddress())

	configurationNodes := first.NodesForConfigurationAddress("coder_script.direct")
	require.Len(t, configurationNodes, 1)
	original := configurationNodes[0]
	configurationNodes[0] = tfgraph.NodeID{}
	require.Equal(
		t, original,
		first.NodesForConfigurationAddress("coder_script.direct")[0],
	)

	node, ok := first.Node(original)
	require.True(t, ok)
	require.Equal(t, `"[root] coder_script.direct (expand)"`, node.RawID())
	_, ok = first.Node(tfgraph.NodeID{})
	if original != (tfgraph.NodeID{}) {
		require.False(t, ok)
	}
}

func TestIndexMatchesCapturedTerraformGraphs(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name                   string
		path                   string
		configurationAddresses []string
		instanceAddresses      []string
	}{
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
		{
			name: "Devcontainer",
			path: "../testdata/resources/devcontainer/devcontainer.tfplan.dot",
			configurationAddresses: []string{
				"coder_agent.main",
				"coder_devcontainer.dev1",
			},
		},
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

			rawGraph, err := os.ReadFile(test.path)
			require.NoError(t, err)
			index, err := tfgraph.Parse(t.Context(), string(rawGraph))
			require.NoError(t, err)
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
