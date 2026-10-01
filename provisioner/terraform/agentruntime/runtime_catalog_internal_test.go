package agentruntime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform/tfgraph"
)

func TestRuntimeCatalogIndexesInstances(t *testing.T) {
	t.Parallel()

	targets := []Target{
		{Kind: KindWorkspaceAgent, Address: `module.runtime["west"].coder_agent.main[1]`},
		{Kind: KindWorkspaceAgent, Address: `module.runtime["east"].coder_agent.main[0]`},
		{Kind: KindWorkspaceAgent, Address: `module.runtime["east"].coder_agent.main[0]`},
		{Kind: KindDevcontainer, Address: `coder_devcontainer.repo["api"]`},
		{Kind: KindDevcontainer, Address: `coder_devcontainer.repo["worker"]`},
	}
	catalog, err := newRuntimeCatalog(t.Context(), targets)
	require.NoError(t, err)
	require.Equal(t, []string{
		`coder_devcontainer.repo["api"]`,
		`coder_devcontainer.repo["worker"]`,
		`module.runtime["east"].coder_agent.main[0]`,
		`module.runtime["west"].coder_agent.main[1]`,
	}, runtimeCatalogTestAddresses(catalog, runtimeCatalogTestIDs(catalog)))

	graph, err := tfgraph.Parse(t.Context(), `digraph {
		"[root] module.runtime.coder_agent.main (expand)"
		"[root] module.runtime[\"east\"].coder_agent.main[0]"
		"[root] coder_devcontainer.repo (expand)"
		"[root] coder_devcontainer.repo[\"worker\"]"
	}`)
	require.NoError(t, err)

	agentConfigNode := runtimeCatalogTestNode(
		t, graph, graph.NodesForConfigurationAddress("module.runtime.coder_agent.main"),
	)
	require.Equal(t, []string{
		`module.runtime["east"].coder_agent.main[0]`,
		`module.runtime["west"].coder_agent.main[1]`,
	}, runtimeCatalogTestAddresses(
		catalog, catalog.runtimesForGraphNode(agentConfigNode),
	))

	agentInstanceNode := runtimeCatalogTestNode(
		t, graph, graph.NodesForInstanceAddress(`module.runtime["east"].coder_agent.main[0]`),
	)
	require.Equal(t, []string{
		`module.runtime["east"].coder_agent.main[0]`,
	}, runtimeCatalogTestAddresses(
		catalog, catalog.runtimesForGraphNode(agentInstanceNode),
	))

	devcontainerConfigNode := runtimeCatalogTestNode(
		t, graph, graph.NodesForConfigurationAddress("coder_devcontainer.repo"),
	)
	require.Equal(t, []string{
		`coder_devcontainer.repo["api"]`,
		`coder_devcontainer.repo["worker"]`,
	}, runtimeCatalogTestAddresses(
		catalog, catalog.runtimesForGraphNode(devcontainerConfigNode),
	))

	devcontainerInstanceNode := runtimeCatalogTestNode(
		t, graph, graph.NodesForInstanceAddress(`coder_devcontainer.repo["worker"]`),
	)
	require.Equal(t, []string{
		`coder_devcontainer.repo["worker"]`,
	}, runtimeCatalogTestAddresses(
		catalog, catalog.runtimesForGraphNode(devcontainerInstanceNode),
	))
}

func TestRuntimeCatalogRejectsInvalidOrExcessiveInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		targets       []Target
		entryLimit    int
		byteLimit     int
		errorContains string
	}{
		{
			name: "Entries",
			targets: []Target{
				{Kind: KindWorkspaceAgent, Address: "coder_agent.first"},
				{Kind: KindWorkspaceAgent, Address: "coder_agent.second"},
			},
			entryLimit:    1,
			byteLimit:     1024,
			errorContains: "limit of 1 agent runtime instances",
		},
		{
			name:          "AddressBytes",
			targets:       []Target{{Kind: KindWorkspaceAgent, Address: "coder_agent.main"}},
			entryLimit:    1,
			byteLimit:     len("coder_agent.main") - 1,
			errorContains: "agent runtime address bytes",
		},
		{
			name:          "WrongAgentType",
			targets:       []Target{{Kind: KindWorkspaceAgent, Address: "coder_devcontainer.repo"}},
			entryLimit:    1,
			byteLimit:     1024,
			errorContains: `expected "coder_agent"`,
		},
		{
			name:          "MalformedAddress",
			targets:       []Target{{Kind: KindWorkspaceAgent, Address: "coder_agent.main["}},
			entryLimit:    1,
			byteLimit:     1024,
			errorContains: "parse agent runtime address",
		},
		{
			name:          "WrongDevcontainerType",
			targets:       []Target{{Kind: KindDevcontainer, Address: "coder_agent.main"}},
			entryLimit:    1,
			byteLimit:     1024,
			errorContains: `expected "coder_devcontainer"`,
		},
		{
			name:          "UnknownKind",
			targets:       []Target{{Kind: 99, Address: "coder_agent.main"}},
			entryLimit:    1,
			byteLimit:     1024,
			errorContains: "unknown agent runtime kind 99",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := newRuntimeCatalogWithLimits(
				t.Context(), test.targets, test.entryLimit, test.byteLimit,
			)
			require.ErrorContains(t, err, test.errorContains)
		})
	}
}

func TestRuntimeCatalogHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newRuntimeCatalog(ctx, []Target{
		{Kind: KindWorkspaceAgent, Address: "coder_agent.main"},
	})
	require.ErrorIs(t, err, context.Canceled)
}

func runtimeCatalogTestIDs(catalog *runtimeCatalog) []runtimeID {
	ids := make([]runtimeID, len(catalog.runtimes))
	for index := range catalog.runtimes {
		ids[index] = runtimeID(index)
	}
	return ids
}

func runtimeCatalogTestAddresses(
	catalog *runtimeCatalog,
	ids []runtimeID,
) []string {
	addresses := make([]string, 0, len(ids))
	for _, id := range ids {
		addresses = append(addresses, catalog.runtimes[id].target.Address)
	}
	return addresses
}

func runtimeCatalogTestNode(
	t *testing.T,
	index *tfgraph.Index,
	ids []tfgraph.NodeID,
) tfgraph.Node {
	t.Helper()
	require.Len(t, ids, 1)
	node, ok := index.Node(ids[0])
	require.True(t, ok)
	return node
}
