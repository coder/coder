// Package unit_test provides tests for the unit package.
//
// DOT Graph Testing:
// The graph tests use golden files for DOT representation verification.
// To update the golden files:
// make gen/golden-files
//
// The golden files contain the expected DOT representation and can be easily
// inspected, version controlled, and updated when the graph structure changes.
package unit_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/unit"
)

// UpdateGoldenFiles indicates golden files should be updated.
// To update the golden files:
// make gen/golden-files
var UpdateGoldenFiles = flag.Bool("update", false, "update .golden files")

// assertDOTGraph asserts that the graph's DOT representation matches the golden file
func assertDOTGraph(t *testing.T, graph *unit.Graph[unit.Status, *unit.Unit], goldenName string) {
	t.Helper()

	dot, err := graph.ToDOT(goldenName)
	require.NoError(t, err)

	goldenFile := filepath.Join("testdata", goldenName+".golden")
	if *UpdateGoldenFiles {
		t.Logf("update golden file for: %q: %s", goldenName, goldenFile)
		err := os.MkdirAll(filepath.Dir(goldenFile), 0o755)
		require.NoError(t, err, "want no error creating golden file directory")
		err = os.WriteFile(goldenFile, []byte(dot), 0o600)
		require.NoError(t, err, "update golden file")
	}

	expected, err := os.ReadFile(goldenFile)
	require.NoError(t, err, "read golden file, run \"make gen/golden-files\" and commit the changes")

	require.Equal(t, string(expected), dot, "golden file mismatch (-want +got): %s, run \"make gen/golden-files\", verify and commit the changes", goldenFile)
}

func TestGraph(t *testing.T) {
	t.Parallel()

	t.Run("ForwardAndReverseEdges", func(t *testing.T) {
		t.Parallel()

		graph := &unit.Graph[unit.Status, *unit.Unit]{}
		unit1 := &unit.Unit{Name: "unit1", Status: unit.StatusPending}
		unit2 := &unit.Unit{Name: "unit2", Status: unit.StatusPending}
		unit3 := &unit.Unit{Name: "unit3", Status: unit.StatusPending}
		err := graph.AddEdge(unit1, unit2, unit.StatusCompleted)
		require.NoError(t, err)
		err = graph.AddEdge(unit1, unit3, unit.StatusStarted)
		require.NoError(t, err)

		// Check for forward edge
		vertices := graph.GetForwardAdjacentVertices(unit1)
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

		// Assert on the DOT representation using golden file
		assertDOTGraph(t, graph, "ForwardAndReverseEdges")
	})

	t.Run("SelfReference", func(t *testing.T) {
		t.Parallel()

		graph := &unit.Graph[unit.Status, *unit.Unit]{}
		unit1 := &unit.Unit{Name: "unit1", Status: unit.StatusPending}
		err := graph.AddEdge(unit1, unit1, unit.StatusCompleted)
		require.Error(t, err)
		require.ErrorContains(t, err, fmt.Sprintf("adding edge (%v -> %v) would create a cycle", unit1, unit1))

		// Assert on the DOT representation using golden file (should be empty graph)
		assertDOTGraph(t, graph, "SelfReference")
	})

	t.Run("Cycle", func(t *testing.T) {
		t.Parallel()

		graph := &unit.Graph[unit.Status, *unit.Unit]{}
		unit1 := &unit.Unit{Name: "unit1", Status: unit.StatusPending}
		unit2 := &unit.Unit{Name: "unit2", Status: unit.StatusPending}
		err := graph.AddEdge(unit1, unit2, unit.StatusCompleted)
		require.NoError(t, err)
		err = graph.AddEdge(unit2, unit1, unit.StatusStarted)
		require.Error(t, err)
		require.ErrorContains(t, err, fmt.Sprintf("adding edge (%v -> %v) would create a cycle", unit2, unit1))

		// Assert on the DOT representation using golden file (should contain only the first edge)
		assertDOTGraph(t, graph, "Cycle")
	})
}
