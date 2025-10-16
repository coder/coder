package unit_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/unit"
)

func TestDependencyCoordinator(t *testing.T) {
	t.Parallel()

	t.Run("ForwardAndReverseEdges", func(t *testing.T) {
		t.Parallel()

		graph := unit.NewDependencyCoordinator()
		unit1 := &unit.Unit{Name: "unit1", Status: unit.StatusPending}
		unit2 := &unit.Unit{Name: "unit2", Status: unit.StatusPending}
		unit3 := &unit.Unit{Name: "unit3", Status: unit.StatusPending}
		graph.AddEdge(unit1, unit2, unit.StatusCompleted)
		graph.AddEdge(unit1, unit3, unit.StatusStarted)

		// Check for forward edge
		vertices := graph.GetAdjacentVertices(unit1)
		require.Len(t, vertices, 2)
		// Unit 1 depends on the completion of Unit2
		require.Contains(t, vertices, unit.DependencyEdge{
			From: unit1,
			To:   unit2,
			Edge: unit.StatusCompleted,
		})
		// Unit 1 depends on the start of Unit3
		require.Contains(t, vertices, unit.DependencyEdge{
			From: unit1,
			To:   unit3,
			Edge: unit.StatusStarted,
		})

		// Check for reverse edges
		unit2ReverseEdges := graph.GetReverseAdjacentVertices(unit2)
		require.Len(t, unit2ReverseEdges, 1)
		// Unit 2 must be completed before Unit 1 can start
		require.Contains(t, unit2ReverseEdges, unit.DependencyEdge{
			From: unit1,
			To:   unit2,
			Edge: unit.StatusCompleted,
		})

		unit3ReverseEdges := graph.GetReverseAdjacentVertices(unit3)
		require.Len(t, unit3ReverseEdges, 1)
		// Unit 3 must be started before Unit 1 can complete
		require.Contains(t, unit3ReverseEdges, unit.DependencyEdge{
			From: unit1,
			To:   unit3,
			Edge: unit.StatusStarted,
		})
	})
}
