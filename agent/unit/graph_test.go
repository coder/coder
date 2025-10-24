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
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/cryptorand"
)

type testGraphEdge string

const (
	testEdgeStarted   testGraphEdge = "started"
	testEdgeCompleted testGraphEdge = "completed"
)

type testGraphVertex struct {
	Name string
}

type (
	testGraph = unit.Graph[testGraphEdge, *testGraphVertex]
	testEdge  = unit.Edge[testGraphEdge, *testGraphVertex]
)

// randInt generates a random integer in the range [0, limit).
func randInt(limit int) int {
	if limit <= 0 {
		return 0
	}
	n, err := cryptorand.Int63n(int64(limit))
	if err != nil {
		return 0
	}
	return int(n)
}

// UpdateGoldenFiles indicates golden files should be updated.
// To update the golden files:
// make gen/golden-files
var UpdateGoldenFiles = flag.Bool("update", false, "update .golden files")

// assertDOTGraph requires that the graph's DOT representation matches the golden file
func assertDOTGraph(t *testing.T, graph *testGraph, goldenName string) {
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

	// Normalize line endings for cross-platform compatibility
	expected = normalizeLineEndings(expected)
	normalizedDot := normalizeLineEndings([]byte(dot))

	assert.Empty(t, cmp.Diff(string(expected), string(normalizedDot)), "golden file mismatch (-want +got): %s, run \"make gen/golden-files\", verify and commit the changes", goldenFile)
}

// normalizeLineEndings ensures that all line endings are normalized to \n.
// Required for Windows compatibility.
func normalizeLineEndings(content []byte) []byte {
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	content = bytes.ReplaceAll(content, []byte("\r"), []byte("\n"))
	return content
}

func TestGraph(t *testing.T) {
	t.Parallel()

	testFuncs := map[string]func(t *testing.T) *unit.Graph[testGraphEdge, *testGraphVertex]{
		"ForwardAndReverseEdges": func(t *testing.T) *unit.Graph[testGraphEdge, *testGraphVertex] {
			graph := &unit.Graph[testGraphEdge, *testGraphVertex]{}
			unit1 := &testGraphVertex{Name: "unit1"}
			unit2 := &testGraphVertex{Name: "unit2"}
			unit3 := &testGraphVertex{Name: "unit3"}
			err := graph.AddEdge(unit1, unit2, testEdgeCompleted)
			require.NoError(t, err)
			err = graph.AddEdge(unit1, unit3, testEdgeStarted)
			require.NoError(t, err)

			// Check for forward edge
			vertices := graph.GetForwardAdjacentVertices(unit1)
			require.Len(t, vertices, 2)
			// Unit 1 depends on the completion of Unit2
			require.Contains(t, vertices, testEdge{
				From: unit1,
				To:   unit2,
				Edge: testEdgeCompleted,
			})
			// Unit 1 depends on the start of Unit3
			require.Contains(t, vertices, testEdge{
				From: unit1,
				To:   unit3,
				Edge: testEdgeStarted,
			})

			// Check for reverse edges
			unit2ReverseEdges := graph.GetReverseAdjacentVertices(unit2)
			require.Len(t, unit2ReverseEdges, 1)
			// Unit 2 must be completed before Unit 1 can start
			require.Contains(t, unit2ReverseEdges, testEdge{
				From: unit1,
				To:   unit2,
				Edge: testEdgeCompleted,
			})

			unit3ReverseEdges := graph.GetReverseAdjacentVertices(unit3)
			require.Len(t, unit3ReverseEdges, 1)
			// Unit 3 must be started before Unit 1 can complete
			require.Contains(t, unit3ReverseEdges, testEdge{
				From: unit1,
				To:   unit3,
				Edge: testEdgeStarted,
			})

			return graph
		},
		"SelfReference": func(t *testing.T) *testGraph {
			graph := &testGraph{}
			unit1 := &testGraphVertex{Name: "unit1"}
			err := graph.AddEdge(unit1, unit1, testEdgeCompleted)
			require.Error(t, err)
			require.ErrorContains(t, err, fmt.Sprintf("adding edge (%v -> %v) would create a cycle", unit1, unit1))

			return graph
		},
		"Cycle": func(t *testing.T) *testGraph {
			graph := &testGraph{}
			unit1 := &testGraphVertex{Name: "unit1"}
			unit2 := &testGraphVertex{Name: "unit2"}
			err := graph.AddEdge(unit1, unit2, testEdgeCompleted)
			require.NoError(t, err)
			err = graph.AddEdge(unit2, unit1, testEdgeStarted)
			require.Error(t, err)
			require.ErrorContains(t, err, fmt.Sprintf("adding edge (%v -> %v) would create a cycle", unit2, unit1))

			return graph
		},
		"MultipleDependenciesSameStatus": func(t *testing.T) *testGraph {
			graph := &testGraph{}
			unit1 := &testGraphVertex{Name: "unit1"}
			unit2 := &testGraphVertex{Name: "unit2"}
			unit3 := &testGraphVertex{Name: "unit3"}
			unit4 := &testGraphVertex{Name: "unit4"}

			// Unit1 depends on completion of both unit2 and unit3 (same status type)
			err := graph.AddEdge(unit1, unit2, testEdgeCompleted)
			require.NoError(t, err)
			err = graph.AddEdge(unit1, unit3, testEdgeCompleted)
			require.NoError(t, err)

			// Unit1 also depends on starting of unit4 (different status type)
			err = graph.AddEdge(unit1, unit4, testEdgeStarted)
			require.NoError(t, err)

			// Check that unit1 has 3 forward dependencies
			forwardEdges := graph.GetForwardAdjacentVertices(unit1)
			require.Len(t, forwardEdges, 3)

			// Verify all expected dependencies exist
			expectedDependencies := []testEdge{
				{From: unit1, To: unit2, Edge: testEdgeCompleted},
				{From: unit1, To: unit3, Edge: testEdgeCompleted},
				{From: unit1, To: unit4, Edge: testEdgeStarted},
			}

			for _, expected := range expectedDependencies {
				require.Contains(t, forwardEdges, expected)
			}

			// Check reverse dependencies
			unit2ReverseEdges := graph.GetReverseAdjacentVertices(unit2)
			require.Len(t, unit2ReverseEdges, 1)
			require.Contains(t, unit2ReverseEdges, testEdge{
				From: unit1, To: unit2, Edge: testEdgeCompleted,
			})

			unit3ReverseEdges := graph.GetReverseAdjacentVertices(unit3)
			require.Len(t, unit3ReverseEdges, 1)
			require.Contains(t, unit3ReverseEdges, testEdge{
				From: unit1, To: unit3, Edge: testEdgeCompleted,
			})

			unit4ReverseEdges := graph.GetReverseAdjacentVertices(unit4)
			require.Len(t, unit4ReverseEdges, 1)
			require.Contains(t, unit4ReverseEdges, testEdge{
				From: unit1, To: unit4, Edge: testEdgeStarted,
			})

			return graph
		},
	}

	for testName, testFunc := range testFuncs {
		var graph *testGraph
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			graph = testFunc(t)
			assertDOTGraph(t, graph, testName)
		})
	}
}

func TestGraphThreadSafety(t *testing.T) {
	t.Parallel()

	t.Run("ConcurrentReadWrite", func(t *testing.T) {
		t.Parallel()

		graph := &testGraph{}
		var wg sync.WaitGroup
		const numWriters = 50
		const numReaders = 100
		const operationsPerWriter = 1000
		const operationsPerReader = 2000

		barrier := make(chan struct{})
		// Launch writers
		for i := 0; i < numWriters; i++ {
			wg.Add(1)
			go func(writerID int) {
				defer wg.Done()
				<-barrier
				for j := 0; j < operationsPerWriter; j++ {
					from := &testGraphVertex{Name: fmt.Sprintf("writer-%d-%d", writerID, j)}
					to := &testGraphVertex{Name: fmt.Sprintf("writer-%d-%d", writerID, j+1)}
					graph.AddEdge(from, to, testEdgeCompleted)
				}
			}(i)
		}

		// Launch readers
		readerResults := make([]struct {
			panicked  bool
			readCount int
		}, numReaders)

		for i := 0; i < numReaders; i++ {
			wg.Add(1)
			go func(readerID int) {
				defer wg.Done()
				<-barrier
				defer func() {
					if r := recover(); r != nil {
						readerResults[readerID].panicked = true
					}
				}()

				readCount := 0
				for j := 0; j < operationsPerReader; j++ {
					// Create a test vertex and read
					testUnit := &testGraphVertex{Name: fmt.Sprintf("test-reader-%d-%d", readerID, j)}
					forwardEdges := graph.GetForwardAdjacentVertices(testUnit)
					reverseEdges := graph.GetReverseAdjacentVertices(testUnit)

					// Just verify no panics (results may be nil for non-existent vertices)
					_ = forwardEdges
					_ = reverseEdges
					readCount++
				}
				readerResults[readerID].readCount = readCount
			}(i)
		}

		close(barrier)
		wg.Wait()

		// Verify no panics occurred in readers
		for i, result := range readerResults {
			require.False(t, result.panicked, "reader %d panicked", i)
			require.Equal(t, operationsPerReader, result.readCount, "reader %d should have performed expected reads", i)
		}
	})

	t.Run("ConcurrentCycleDetection", func(t *testing.T) {
		t.Parallel()

		graph := &testGraph{}

		// Pre-create chain: A→B→C→D
		unitA := &testGraphVertex{Name: "A"}
		unitB := &testGraphVertex{Name: "B"}
		unitC := &testGraphVertex{Name: "C"}
		unitD := &testGraphVertex{Name: "D"}

		err := graph.AddEdge(unitA, unitB, testEdgeCompleted)
		require.NoError(t, err)
		err = graph.AddEdge(unitB, unitC, testEdgeCompleted)
		require.NoError(t, err)
		err = graph.AddEdge(unitC, unitD, testEdgeCompleted)
		require.NoError(t, err)

		barrier := make(chan struct{})
		var wg sync.WaitGroup
		const numGoroutines = 50
		cycleErrors := make([]error, numGoroutines)

		// Launch goroutines trying to add D→A (creates cycle)
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				<-barrier
				err := graph.AddEdge(unitD, unitA, testEdgeCompleted)
				cycleErrors[goroutineID] = err
			}(i)
		}

		close(barrier)
		wg.Wait()

		// Verify all attempts correctly returned cycle error
		for i, err := range cycleErrors {
			require.Error(t, err, "goroutine %d should have detected cycle", i)
			require.Contains(t, err.Error(), "would create a cycle")
		}

		// Verify graph remains valid (original chain intact)
		dot, err := graph.ToDOT("test")
		require.NoError(t, err)
		require.NotEmpty(t, dot)
	})

	t.Run("ConcurrentToDOT", func(t *testing.T) {
		t.Parallel()

		graph := &testGraph{}

		// Pre-populate graph
		for i := 0; i < 20; i++ {
			from := &testGraphVertex{Name: fmt.Sprintf("dot-unit-%d", i)}
			to := &testGraphVertex{Name: fmt.Sprintf("dot-unit-%d", i+1)}
			err := graph.AddEdge(from, to, testEdgeCompleted)
			require.NoError(t, err)
		}

		barrier := make(chan struct{})
		var wg sync.WaitGroup
		const numReaders = 100
		const numWriters = 20
		dotResults := make([]string, numReaders)

		// Launch readers calling ToDOT
		dotErrors := make([]error, numReaders)
		for i := 0; i < numReaders; i++ {
			wg.Add(1)
			go func(readerID int) {
				defer wg.Done()
				<-barrier
				dot, err := graph.ToDOT(fmt.Sprintf("test-%d", readerID))
				dotErrors[readerID] = err
				if err == nil {
					dotResults[readerID] = dot
				}
			}(i)
		}

		// Launch writers adding edges
		for i := 0; i < numWriters; i++ {
			wg.Add(1)
			go func(writerID int) {
				defer wg.Done()
				<-barrier
				from := &testGraphVertex{Name: fmt.Sprintf("writer-dot-%d", writerID)}
				to := &testGraphVertex{Name: fmt.Sprintf("writer-dot-target-%d", writerID)}
				graph.AddEdge(from, to, testEdgeCompleted)
			}(i)
		}

		close(barrier)
		wg.Wait()

		// Verify no errors occurred during DOT generation
		for i, err := range dotErrors {
			require.NoError(t, err, "DOT generation error at index %d", i)
		}

		// Verify all DOT results are valid
		for i, dot := range dotResults {
			require.NotEmpty(t, dot, "DOT result %d should not be empty", i)
		}
	})
}

func BenchmarkGraph_ConcurrentMixedOperations(b *testing.B) {
	graph := &testGraph{}
	var wg sync.WaitGroup
	const numGoroutines = 200

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Launch goroutines performing random operations
		for j := 0; j < numGoroutines; j++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				operationCount := 0

				for operationCount < 50 {
					operation := float32(randInt(100)) / 100.0

					if operation < 0.6 { // 60% reads
						// Read operation
						testUnit := &testGraphVertex{Name: fmt.Sprintf("bench-read-%d-%d", goroutineID, operationCount)}
						forwardEdges := graph.GetForwardAdjacentVertices(testUnit)
						reverseEdges := graph.GetReverseAdjacentVertices(testUnit)

						// Just verify no panics (results may be nil for non-existent vertices)
						_ = forwardEdges
						_ = reverseEdges
					} else { // 40% writes
						// Write operation
						from := &testGraphVertex{Name: fmt.Sprintf("bench-write-%d-%d", goroutineID, operationCount)}
						to := &testGraphVertex{Name: fmt.Sprintf("bench-write-target-%d-%d", goroutineID, operationCount)}
						graph.AddEdge(from, to, testEdgeCompleted)
					}

					operationCount++
				}
			}(j)
		}

		wg.Wait()
	}
}
