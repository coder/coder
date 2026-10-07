package tfgraph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueryBoundsWork(t *testing.T) {
	t.Parallel()

	sourceGraph, err := Parse(t.Context(), `digraph {
		"[root] local.bridge.attribute (expand)"
		"[root] coder_agent.main (expand)"
		"[root] coder_agent.other (expand)"
		"[root] local.bridge.attribute (expand)" -> "[root] coder_agent.main (expand)"
	}`)
	require.NoError(t, err)

	t.Run("ReferenceLookups", func(t *testing.T) {
		t.Parallel()

		query, err := newQueryWithLimits(sourceGraph, queryLimits{
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

	t.Run("ResolverReferenceLookupsShareBudget", func(t *testing.T) {
		t.Parallel()

		resolver := func(
			_ context.Context,
			lookup ReferenceLookup,
			_ Node,
			_ []NodeID,
		) ([]NodeID, error) {
			return lookup("", []string{"coder_agent.other"})
		}
		resolvedGraph, err := sourceGraph.WithResolvedDependencies(resolver)
		require.NoError(t, err)
		query, err := newQueryWithLimits(resolvedGraph, queryLimits{
			referenceLookups: 0,
			nodeVisits:       defaultQueryLimits().nodeVisits,
			traversedEdges:   defaultQueryLimits().traversedEdges,
		})
		require.NoError(t, err)
		_, err = query.ReachableBoundaryNodes(
			t.Context(),
			sourceGraph.NodesForConfigurationAddress("local.bridge.attribute"),
			func(Node) bool { return false },
		)
		require.ErrorContains(t, err, "Terraform graph reference lookups")
	})

	t.Run("ResolverReferenceLookupsUseTraversalContext", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		resolver := func(
			_ context.Context,
			lookup ReferenceLookup,
			_ Node,
			_ []NodeID,
		) ([]NodeID, error) {
			cancel()
			_, err := lookup("", []string{"coder_agent.other"})
			require.ErrorIs(t, err, context.Canceled)
			return nil, err
		}
		resolvedGraph, err := sourceGraph.WithResolvedDependencies(resolver)
		require.NoError(t, err)
		query, err := newQueryWithLimits(resolvedGraph, defaultQueryLimits())
		require.NoError(t, err)
		_, err = query.ReachableBoundaryNodes(
			ctx,
			sourceGraph.NodesForConfigurationAddress("local.bridge.attribute"),
			func(Node) bool { return false },
		)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("DuplicateReferencesConsumeWorkOnce", func(t *testing.T) {
		t.Parallel()

		query, err := newQueryWithLimits(sourceGraph, queryLimits{
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
			sourceGraph.NodesForConfigurationAddress("local.bridge.attribute"),
			nodes,
		)
		require.Equal(t, 1, query.referenceLookups)
		require.Equal(t, 1, query.nodeVisits)
	})

	t.Run("MultipleMatchingNodesCountTowardVisitLimit", func(t *testing.T) {
		t.Parallel()

		matchingIndex := &graphIndex{
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
		matchingGraph := &Graph{index: matchingIndex}
		query, err := newQueryWithLimits(
			matchingGraph, queryLimits{
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

	t.Run("DuplicateStartNodesConsumeWorkOnce", func(t *testing.T) {
		t.Parallel()

		start := sourceGraph.NodesForConfigurationAddress("local.bridge.attribute")
		require.Len(t, start, 1)
		query, err := newQueryWithLimits(sourceGraph, queryLimits{
			referenceLookups: defaultQueryLimits().referenceLookups,
			nodeVisits:       1,
			traversedEdges:   defaultQueryLimits().traversedEdges,
		})
		require.NoError(t, err)
		boundaries, err := query.ReachableBoundaryNodes(
			t.Context(), []NodeID{start[0], start[0]},
			func(Node) bool { return true },
		)
		require.NoError(t, err)
		require.Equal(t, start, boundaries)
		require.Equal(t, 1, query.nodeVisits)
	})

	t.Run("ConvergingPathsVisitNodeOnce", func(t *testing.T) {
		t.Parallel()

		converging, err := Parse(t.Context(), `digraph {
			"[root] local.start" -> "[root] local.left"
			"[root] local.start" -> "[root] local.right"
			"[root] local.left" -> "[root] local.merge"
			"[root] local.right" -> "[root] local.merge"
			"[root] local.merge" -> "[root] coder_agent.main"
		}`)
		require.NoError(t, err)
		start := converging.NodesForInstanceAddress("local.start")
		require.Len(t, start, 1)
		boundary := converging.NodesForInstanceAddress("coder_agent.main")
		require.Len(t, boundary, 1)

		// The budget permits each graph edge once. Requeuing local.merge
		// would traverse its outgoing edge twice and exceed the limit.
		const edgeCount = 5
		query, err := newQueryWithLimits(
			converging, queryLimits{
				referenceLookups: defaultQueryLimits().referenceLookups,
				nodeVisits:       defaultQueryLimits().nodeVisits,
				traversedEdges:   edgeCount,
			})
		require.NoError(t, err)
		boundaries, err := query.ReachableBoundaryNodes(
			t.Context(), start,
			func(node Node) bool {
				return node.InstanceAddress() == "coder_agent.main"
			},
		)
		require.NoError(t, err)
		require.Equal(t, boundary, boundaries)
		require.Equal(t, edgeCount, query.traversedEdges)
	})

	t.Run("TraversedNodesCountTowardVisitLimit", func(t *testing.T) {
		t.Parallel()

		chain, err := Parse(t.Context(), `digraph {
			"[root] local.a" -> "[root] local.b"
			"[root] local.b" -> "[root] local.c"
			"[root] local.c" -> "[root] local.d"
		}`)
		require.NoError(t, err)
		start := chain.NodesForInstanceAddress("local.a")
		require.Len(t, start, 1)
		query, err := newQueryWithLimits(chain, queryLimits{
			referenceLookups: defaultQueryLimits().referenceLookups,
			nodeVisits:       1,
			traversedEdges:   defaultQueryLimits().traversedEdges,
		})
		require.NoError(t, err)
		_, err = query.ReachableBoundaryNodes(
			t.Context(), start, func(Node) bool { return false },
		)
		require.ErrorContains(t, err, "Terraform graph node visits")
	})

	t.Run("TraversedEdges", func(t *testing.T) {
		t.Parallel()

		query, err := newQueryWithLimits(sourceGraph, queryLimits{
			referenceLookups: defaultQueryLimits().referenceLookups,
			nodeVisits:       defaultQueryLimits().nodeVisits,
			traversedEdges:   0,
		})
		require.NoError(t, err)
		_, err = query.ReachableBoundaryNodes(
			t.Context(),
			sourceGraph.NodesForConfigurationAddress("local.bridge.attribute"),
			func(Node) bool { return false },
		)
		require.ErrorContains(t, err, "Terraform graph traversed edges")
	})

	t.Run("AddedDependenciesCountTowardEdgeLimit", func(t *testing.T) {
		t.Parallel()

		additional := sourceGraph.NodesForConfigurationAddress("coder_agent.other")
		require.Len(t, additional, 1)
		resolver := func(
			_ context.Context,
			_ ReferenceLookup,
			source Node,
			graphDependencies []NodeID,
		) ([]NodeID, error) {
			if source.ConfigurationAddress() != "local.bridge.attribute" {
				return graphDependencies, nil
			}
			return append(graphDependencies, additional[0]), nil
		}
		resolvedGraph, err := sourceGraph.WithResolvedDependencies(resolver)
		require.NoError(t, err)
		query, err := newQueryWithLimits(resolvedGraph, queryLimits{
			referenceLookups: defaultQueryLimits().referenceLookups,
			nodeVisits:       defaultQueryLimits().nodeVisits,
			traversedEdges:   1,
		})
		require.NoError(t, err)
		_, err = query.ReachableBoundaryNodes(
			t.Context(),
			sourceGraph.NodesForConfigurationAddress("local.bridge.attribute"),
			func(Node) bool { return false },
		)
		require.ErrorContains(t, err, "Terraform graph traversed edges")
	})
}

func TestQueryAccumulatesWorkPerRequest(t *testing.T) {
	t.Parallel()

	graph, err := Parse(t.Context(), `digraph {
		"[root] local.bridge.attribute (expand)"
		"[root] coder_agent.main (expand)"
		"[root] local.bridge.attribute (expand)" -> "[root] coder_agent.main (expand)"
	}`)
	require.NoError(t, err)
	start := graph.NodesForConfigurationAddress("local.bridge.attribute")
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

			query, err := newQueryWithLimits(graph, test.limits)
			require.NoError(t, err)
			require.NoError(t, test.run(t, query))
			require.ErrorContains(t, test.run(t, query), test.errorContains)

			freshQuery, err := newQueryWithLimits(graph, test.limits)
			require.NoError(t, err)
			require.NoError(t, test.run(t, freshQuery))
		})
	}
}
