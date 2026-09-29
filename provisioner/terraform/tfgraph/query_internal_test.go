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
			nodeVisits:       defaultQueryLimits().nodeVisits,
			traversedEdges:   defaultQueryLimits().traversedEdges,
		})
		require.NoError(t, err)
		_, err = query.ConfigurationNodesForReferences(
			t.Context(), "", []string{"local.bridge"},
		)
		require.ErrorContains(t, err, "Terraform graph reference lookups")
	})

	t.Run("DuplicateReferencesConsumeWorkOnce", func(t *testing.T) {
		t.Parallel()

		query, err := newQueryWithLimits(index, queryLimits{
			referenceLookups: 1,
			nodeVisits:       1,
			traversedEdges:   defaultQueryLimits().traversedEdges,
		})
		require.NoError(t, err)
		nodes, err := query.ConfigurationNodesForReferences(
			t.Context(), "", []string{
				"local.bridge.attribute",
				"local.bridge.attribute",
			},
		)
		require.NoError(t, err)
		require.Equal(
			t,
			index.NodesForConfigurationAddress("local.bridge.attribute"),
			nodes,
		)
		require.Equal(t, 1, query.referenceLookups)
		require.Equal(t, 1, query.nodeVisits)
	})

	t.Run("MultipleMatchingNodesCountTowardVisitLimit", func(t *testing.T) {
		t.Parallel()

		matchingIndex := &Index{
			nodes: []Node{
				{address: "local.bridge.attribute", operation: "expand"},
				{address: "local.bridge.attribute", operation: "expand"},
			},
			dependencies: make([][]NodeID, 2),
		}
		matchingIndex.nodesByConfigurationAddress = map[string][]NodeID{
			"local.bridge.attribute": {
				nodeID(matchingIndex, 0),
				nodeID(matchingIndex, 1),
			},
		}
		query, err := newQueryWithLimits(matchingIndex, queryLimits{
			referenceLookups: defaultQueryLimits().referenceLookups,
			nodeVisits:       1,
			traversedEdges:   defaultQueryLimits().traversedEdges,
		})
		require.NoError(t, err)
		_, err = query.ConfigurationNodesForReferences(
			t.Context(), "", []string{"local.bridge.attribute"},
		)
		require.ErrorContains(t, err, "Terraform graph node visits")
	})

	t.Run("DuplicateStartNodesCountTowardVisitLimit", func(t *testing.T) {
		t.Parallel()

		start := index.NodesForConfigurationAddress("local.bridge.attribute")
		require.Len(t, start, 1)
		query, err := newQueryWithLimits(index, queryLimits{
			referenceLookups: defaultQueryLimits().referenceLookups,
			nodeVisits:       1,
			traversedEdges:   defaultQueryLimits().traversedEdges,
		})
		require.NoError(t, err)
		_, err = query.ReachableBoundaryNodes(
			t.Context(), []NodeID{start[0], start[0]},
			func(Node) bool { return true },
		)
		require.ErrorContains(t, err, "Terraform graph node visits")
	})

	t.Run("TraversedEdges", func(t *testing.T) {
		t.Parallel()

		query, err := newQueryWithLimits(index, queryLimits{
			referenceLookups: defaultQueryLimits().referenceLookups,
			nodeVisits:       defaultQueryLimits().nodeVisits,
			traversedEdges:   0,
		})
		require.NoError(t, err)
		_, err = query.ReachableBoundaryNodes(
			t.Context(),
			index.NodesForConfigurationAddress("local.bridge.attribute"),
			func(Node) bool { return false },
		)
		require.ErrorContains(t, err, "Terraform graph traversed edges")
	})
}

func TestQueryAccumulatesWorkPerRequest(t *testing.T) {
	t.Parallel()

	index, err := Parse(t.Context(), `digraph {
		"[root] local.bridge.attribute (expand)"
		"[root] coder_agent.main (expand)"
		"[root] local.bridge.attribute (expand)" -> "[root] coder_agent.main (expand)"
	}`)
	require.NoError(t, err)
	start := index.NodesForConfigurationAddress("local.bridge.attribute")
	require.Len(t, start, 1)

	for _, test := range []struct {
		name          string
		limits        queryLimits
		run           func(*testing.T, *Query) error
		errorContains string
	}{
		{
			name: "ReferenceLookups",
			limits: queryLimits{
				referenceLookups: 1,
				nodeVisits:       defaultQueryLimits().nodeVisits,
				traversedEdges:   defaultQueryLimits().traversedEdges,
			},
			run: func(t *testing.T, query *Query) error {
				_, err := query.ConfigurationNodesForReferences(
					t.Context(), "", []string{"local.bridge.attribute"},
				)
				return err
			},
			errorContains: "Terraform graph reference lookups",
		},
		{
			name: "NodeVisits",
			limits: queryLimits{
				referenceLookups: defaultQueryLimits().referenceLookups,
				nodeVisits:       1,
				traversedEdges:   defaultQueryLimits().traversedEdges,
			},
			run: func(t *testing.T, query *Query) error {
				_, err := query.ReachableBoundaryNodes(
					t.Context(), start, func(Node) bool { return true },
				)
				return err
			},
			errorContains: "Terraform graph node visits",
		},
		{
			name: "TraversedEdges",
			limits: queryLimits{
				referenceLookups: defaultQueryLimits().referenceLookups,
				nodeVisits:       defaultQueryLimits().nodeVisits,
				traversedEdges:   1,
			},
			run: func(t *testing.T, query *Query) error {
				_, err := query.ReachableBoundaryNodes(
					t.Context(), start, func(Node) bool { return false },
				)
				return err
			},
			errorContains: "Terraform graph traversed edges",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			query, err := newQueryWithLimits(index, test.limits)
			require.NoError(t, err)
			require.NoError(t, test.run(t, query))
			require.ErrorContains(t, test.run(t, query), test.errorContains)

			freshQuery, err := newQueryWithLimits(index, test.limits)
			require.NoError(t, err)
			require.NoError(t, test.run(t, freshQuery))
		})
	}
}
