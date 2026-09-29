package tfgraph

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueryBoundsWork(t *testing.T) {
	t.Parallel()

	index, err := Parse(t.Context(), `digraph {
		"[root] local.bridge.attribute (expand)"
		"[root] coder_agent.main (expand)"
		"[root] local.bridge.attribute (expand)" -> "[root] coder_agent.main (expand)"
	}`)
	require.NoError(t, err)

	t.Run("ReferenceLookups", func(t *testing.T) {
		t.Parallel()

		query, err := newQueryWithLimits(index, queryLimits{
			referenceLookups: 0,
			traversedEdges:   defaultQueryLimits().traversedEdges,
		})
		require.NoError(t, err)
		_, err = query.ConfigurationNodesForReferences(
			t.Context(), "", []string{"local.bridge"},
		)
		require.ErrorContains(t, err, "Terraform graph reference lookups")
	})

	t.Run("TraversedEdges", func(t *testing.T) {
		t.Parallel()

		query, err := newQueryWithLimits(index, queryLimits{
			referenceLookups: defaultQueryLimits().referenceLookups,
			traversedEdges:   0,
		})
		require.NoError(t, err)
		_, err = query.ReachableTerminalNodes(
			t.Context(),
			index.NodesForConfigurationAddress("local.bridge.attribute"),
			func(Node) bool { return false },
		)
		require.ErrorContains(t, err, "Terraform graph traversed edges")
	})
}

func TestQueryRejectsInvalidDependencies(t *testing.T) {
	t.Parallel()

	index := &Index{
		nodes:                       []Node{{address: "local.start"}},
		dependencies:                [][]NodeID{{{}}},
		nodesByConfigurationAddress: map[string][]NodeID{},
		nodesByInstanceAddress:      map[string][]NodeID{},
	}
	query, err := NewQuery(index)
	require.NoError(t, err)
	_, err = query.ReachableTerminalNodes(
		t.Context(), []NodeID{nodeID(index, 0)},
		func(Node) bool { return false },
	)
	require.ErrorContains(t, err, "outside its index")
}
