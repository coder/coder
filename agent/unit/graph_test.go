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
	"crypto/rand"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/unit"
)

// Test types for thread safety testing
type Status string

const (
	StatusPending   Status = "pending"
	StatusStarted   Status = "started"
	StatusCompleted Status = "completed"
)

type Unit struct {
	Name   string
	Status Status
}

// DependencyEdge is an alias for unit.Edge for backward compatibility
type DependencyEdge = unit.Edge[Status, *Unit]

// secureRandInt generates a secure random integer in the range [0, limit)
func secureRandInt(limit int) int {
	if limit <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}

// createTestOutputDir creates a gitignored directory for test outputs
func createTestOutputDir(t *testing.T) string {
	outputDir := filepath.Join("test-output", "thread-safety", t.Name())
	err := os.MkdirAll(outputDir, 0755)
	require.NoError(t, err, "failed to create test output directory")
	return outputDir
}

// saveDOTFile saves a DOT representation of the graph to a file
func saveDOTFile(t *testing.T, graph *unit.Graph[Status, *Unit], filename string) {
	outputDir := createTestOutputDir(t)
	dot, err := graph.ToDOT(filename)
	if err != nil {
		t.Logf("Failed to generate DOT for %s: %v", filename, err)
		return
	}

	dotPath := filepath.Join(outputDir, filename+".dot")
	err = os.WriteFile(dotPath, []byte(dot), 0600)
	require.NoError(t, err, "failed to write DOT file")
	t.Logf("Saved DOT file: %s", dotPath)
}

// UpdateGoldenFiles indicates golden files should be updated.
// To update the golden files:
// make gen/golden-files
var UpdateGoldenFiles = flag.Bool("update", false, "update .golden files")

// assertDOTGraph asserts that the graph's DOT representation matches the golden file
func assertDOTGraph(t *testing.T, graph *unit.Graph[Status, *Unit], goldenName string) {
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

		graph := &unit.Graph[Status, *Unit]{}
		unit1 := &Unit{Name: "unit1", Status: StatusPending}
		unit2 := &Unit{Name: "unit2", Status: StatusPending}
		unit3 := &Unit{Name: "unit3", Status: StatusPending}
		err := graph.AddEdge(unit1, unit2, StatusCompleted)
		require.NoError(t, err)
		err = graph.AddEdge(unit1, unit3, StatusStarted)
		require.NoError(t, err)

		// Check for forward edge
		vertices := graph.GetForwardAdjacentVertices(unit1)
		require.Len(t, vertices, 2)
		// Unit 1 depends on the completion of Unit2
		require.Contains(t, vertices, DependencyEdge{
			From: unit1,
			To:   unit2,
			Edge: StatusCompleted,
		})
		// Unit 1 depends on the start of Unit3
		require.Contains(t, vertices, DependencyEdge{
			From: unit1,
			To:   unit3,
			Edge: StatusStarted,
		})

		// Check for reverse edges
		unit2ReverseEdges := graph.GetReverseAdjacentVertices(unit2)
		require.Len(t, unit2ReverseEdges, 1)
		// Unit 2 must be completed before Unit 1 can start
		require.Contains(t, unit2ReverseEdges, DependencyEdge{
			From: unit1,
			To:   unit2,
			Edge: StatusCompleted,
		})

		unit3ReverseEdges := graph.GetReverseAdjacentVertices(unit3)
		require.Len(t, unit3ReverseEdges, 1)
		// Unit 3 must be started before Unit 1 can complete
		require.Contains(t, unit3ReverseEdges, DependencyEdge{
			From: unit1,
			To:   unit3,
			Edge: StatusStarted,
		})

		// Assert on the DOT representation using golden file
		assertDOTGraph(t, graph, "ForwardAndReverseEdges")
	})

	t.Run("SelfReference", func(t *testing.T) {
		t.Parallel()

		graph := &unit.Graph[Status, *Unit]{}
		unit1 := &Unit{Name: "unit1", Status: StatusPending}
		err := graph.AddEdge(unit1, unit1, StatusCompleted)
		require.Error(t, err)
		require.ErrorContains(t, err, fmt.Sprintf("adding edge (%v -> %v) would create a cycle", unit1, unit1))

		// Assert on the DOT representation using golden file (should be empty graph)
		assertDOTGraph(t, graph, "SelfReference")
	})

	t.Run("Cycle", func(t *testing.T) {
		t.Parallel()

		graph := &unit.Graph[Status, *Unit]{}
		unit1 := &Unit{Name: "unit1", Status: StatusPending}
		unit2 := &Unit{Name: "unit2", Status: StatusPending}
		err := graph.AddEdge(unit1, unit2, StatusCompleted)
		require.NoError(t, err)
		err = graph.AddEdge(unit2, unit1, StatusStarted)
		require.Error(t, err)
		require.ErrorContains(t, err, fmt.Sprintf("adding edge (%v -> %v) would create a cycle", unit2, unit1))

		// Assert on the DOT representation using golden file (should contain only the first edge)
		assertDOTGraph(t, graph, "Cycle")
	})

	t.Run("MultipleDependenciesSameStatus", func(t *testing.T) {
		t.Parallel()

		graph := &unit.Graph[Status, *Unit]{}
		unit1 := &Unit{Name: "unit1", Status: StatusPending}
		unit2 := &Unit{Name: "unit2", Status: StatusPending}
		unit3 := &Unit{Name: "unit3", Status: StatusPending}
		unit4 := &Unit{Name: "unit4", Status: StatusPending}

		// Unit1 depends on completion of both unit2 and unit3 (same status type)
		err := graph.AddEdge(unit1, unit2, StatusCompleted)
		require.NoError(t, err)
		err = graph.AddEdge(unit1, unit3, StatusCompleted)
		require.NoError(t, err)

		// Unit1 also depends on starting of unit4 (different status type)
		err = graph.AddEdge(unit1, unit4, StatusStarted)
		require.NoError(t, err)

		// Check that unit1 has 3 forward dependencies
		forwardEdges := graph.GetForwardAdjacentVertices(unit1)
		require.Len(t, forwardEdges, 3)

		// Verify all expected dependencies exist
		expectedDependencies := []DependencyEdge{
			{From: unit1, To: unit2, Edge: StatusCompleted},
			{From: unit1, To: unit3, Edge: StatusCompleted},
			{From: unit1, To: unit4, Edge: StatusStarted},
		}

		for _, expected := range expectedDependencies {
			require.Contains(t, forwardEdges, expected)
		}

		// Check reverse dependencies
		unit2ReverseEdges := graph.GetReverseAdjacentVertices(unit2)
		require.Len(t, unit2ReverseEdges, 1)
		require.Contains(t, unit2ReverseEdges, DependencyEdge{
			From: unit1, To: unit2, Edge: StatusCompleted,
		})

		unit3ReverseEdges := graph.GetReverseAdjacentVertices(unit3)
		require.Len(t, unit3ReverseEdges, 1)
		require.Contains(t, unit3ReverseEdges, DependencyEdge{
			From: unit1, To: unit3, Edge: StatusCompleted,
		})

		unit4ReverseEdges := graph.GetReverseAdjacentVertices(unit4)
		require.Len(t, unit4ReverseEdges, 1)
		require.Contains(t, unit4ReverseEdges, DependencyEdge{
			From: unit1, To: unit4, Edge: StatusStarted,
		})

		// Assert on the DOT representation using golden file
		assertDOTGraph(t, graph, "MultipleDependenciesSameStatus")
	})
}

func TestGraphThreadSafety(t *testing.T) {
	t.Parallel()

	t.Run("ConcurrentAddEdge", func(t *testing.T) {
		t.Parallel()

		graph := &unit.Graph[Status, *Unit]{}
		var wg sync.WaitGroup
		const numGoroutines = 100
		const edgesPerGoroutine = 10

		// Launch goroutines to add edges concurrently
		errors := make([]error, numGoroutines*edgesPerGoroutine)
		errorIndex := 0
		var mu sync.Mutex

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				for j := 0; j < edgesPerGoroutine; j++ {
					from := &Unit{Name: fmt.Sprintf("unit-%d-%d", goroutineID, j), Status: StatusPending}
					to := &Unit{Name: fmt.Sprintf("unit-%d-%d", goroutineID, j+1), Status: StatusPending}
					err := graph.AddEdge(from, to, StatusCompleted)

					mu.Lock()
					errors[errorIndex] = err
					errorIndex++
					mu.Unlock()
				}
			}(i)
		}

		wg.Wait()

		// Verify no errors occurred during concurrent operations
		for i, err := range errors {
			require.NoError(t, err, "error at index %d", i)
		}

		// Save DOT file for analysis
		saveDOTFile(t, graph, "concurrent-add-edge")

		// Verify no panics occurred and graph is in valid state
		// We can't easily count total edges due to concurrent access, but we can verify
		// the graph is still functional
		dot, err := graph.ToDOT("test")
		require.NoError(t, err)
		require.NotEmpty(t, dot)
	})

	t.Run("ConcurrentReads", func(t *testing.T) {
		t.Parallel()

		graph := &unit.Graph[Status, *Unit]{}

		// Pre-populate graph with 50 vertices and edges
		units := make([]*Unit, 50)
		for i := 0; i < 50; i++ {
			units[i] = &Unit{Name: fmt.Sprintf("unit-%d", i), Status: StatusPending}
		}

		// Add edges to create a chain
		for i := 0; i < 49; i++ {
			err := graph.AddEdge(units[i], units[i+1], StatusCompleted)
			require.NoError(t, err)
		}

		// Save initial DOT file
		saveDOTFile(t, graph, "concurrent-reads-initial")

		var wg sync.WaitGroup
		const numReaders = 200

		// Launch readers
		results := make([]struct {
			forwardLen int
			reverseLen int
			panicked   bool
		}, numReaders)

		for i := 0; i < numReaders; i++ {
			wg.Add(1)
			go func(readerID int) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						results[readerID].panicked = true
					}
				}()

				// Read from random vertices
				vertex := units[secureRandInt(len(units))]
				forwardEdges := graph.GetForwardAdjacentVertices(vertex)
				reverseEdges := graph.GetReverseAdjacentVertices(vertex)

				// Store results for verification outside goroutine
				results[readerID].forwardLen = len(forwardEdges)
				results[readerID].reverseLen = len(reverseEdges)
			}(i)
		}

		wg.Wait()

		// Verify results outside of goroutines
		for i, result := range results {
			require.False(t, result.panicked, "reader %d panicked", i)
			require.LessOrEqual(t, result.forwardLen, 1, "reader %d: chain has max 1 forward edge", i)
			require.LessOrEqual(t, result.reverseLen, 1, "reader %d: chain has max 1 reverse edge", i)
		}
	})

	t.Run("ConcurrentReadWrite", func(t *testing.T) {
		t.Parallel()

		graph := &unit.Graph[Status, *Unit]{}
		var wg sync.WaitGroup
		const numWriters = 50
		const numReaders = 100

		// Launch writers
		for i := 0; i < numWriters; i++ {
			wg.Add(1)
			go func(writerID int) {
				defer wg.Done()
				start := time.Now()
				for time.Since(start) < 100*time.Millisecond {
					from := &Unit{Name: fmt.Sprintf("writer-%d-%d", writerID, time.Now().UnixNano()), Status: StatusPending}
					to := &Unit{Name: fmt.Sprintf("writer-%d-%d", writerID, time.Now().UnixNano()+1), Status: StatusPending}
					graph.AddEdge(from, to, StatusCompleted)
					time.Sleep(time.Microsecond) // Small delay to allow readers
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
				defer func() {
					if r := recover(); r != nil {
						readerResults[readerID].panicked = true
					}
				}()

				start := time.Now()
				readCount := 0
				for time.Since(start) < 100*time.Millisecond {
					// Create a test vertex and read
					testUnit := &Unit{Name: "test-reader", Status: StatusPending}
					forwardEdges := graph.GetForwardAdjacentVertices(testUnit)
					reverseEdges := graph.GetReverseAdjacentVertices(testUnit)

					// Just verify no panics (results may be nil for non-existent vertices)
					_ = forwardEdges
					_ = reverseEdges
					readCount++
					time.Sleep(time.Microsecond)
				}
				readerResults[readerID].readCount = readCount
			}(i)
		}

		wg.Wait()

		// Verify no panics occurred in readers
		for i, result := range readerResults {
			require.False(t, result.panicked, "reader %d panicked", i)
			require.Greater(t, result.readCount, 0, "reader %d should have performed some reads", i)
		}
	})

	t.Run("ConcurrentVertexCreation", func(t *testing.T) {
		t.Parallel()

		graph := &unit.Graph[Status, *Unit]{}
		var wg sync.WaitGroup
		const numGoroutines = 100
		vertexNames := make(map[string]bool)
		var mu sync.Mutex

		// Launch goroutines to create vertices concurrently
		errors := make([]error, numGoroutines*5)
		errorIndex := 0
		var errorMu sync.Mutex

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				for j := 0; j < 5; j++ {
					vertexName := fmt.Sprintf("vertex-%d-%d", goroutineID, j)
					from := &Unit{Name: vertexName, Status: StatusPending}
					to := &Unit{Name: fmt.Sprintf("target-%d-%d", goroutineID, j), Status: StatusPending}

					err := graph.AddEdge(from, to, StatusCompleted)

					errorMu.Lock()
					errors[errorIndex] = err
					errorIndex++
					errorMu.Unlock()

					// Track vertex names to verify uniqueness
					mu.Lock()
					vertexNames[vertexName] = true
					mu.Unlock()
				}
			}(i)
		}

		wg.Wait()

		// Verify no errors occurred during concurrent operations
		for i, err := range errors {
			require.NoError(t, err, "error at index %d", i)
		}

		// Verify all vertices were created and graph is consistent
		require.Equal(t, numGoroutines*5, len(vertexNames))

		// Verify graph can still be exported (no internal corruption)
		dot, err := graph.ToDOT("test")
		require.NoError(t, err)
		require.NotEmpty(t, dot)
	})

	t.Run("ConcurrentCycleDetection", func(t *testing.T) {
		t.Parallel()

		graph := &unit.Graph[Status, *Unit]{}

		// Pre-create chain: A→B→C→D
		unitA := &Unit{Name: "A", Status: StatusPending}
		unitB := &Unit{Name: "B", Status: StatusPending}
		unitC := &Unit{Name: "C", Status: StatusPending}
		unitD := &Unit{Name: "D", Status: StatusPending}

		err := graph.AddEdge(unitA, unitB, StatusCompleted)
		require.NoError(t, err)
		err = graph.AddEdge(unitB, unitC, StatusCompleted)
		require.NoError(t, err)
		err = graph.AddEdge(unitC, unitD, StatusCompleted)
		require.NoError(t, err)

		var wg sync.WaitGroup
		const numGoroutines = 50
		cycleErrors := make([]error, numGoroutines)

		// Launch goroutines trying to add D→A (creates cycle)
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				err := graph.AddEdge(unitD, unitA, StatusCompleted)
				cycleErrors[goroutineID] = err
			}(i)
		}

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

		graph := &unit.Graph[Status, *Unit]{}

		// Pre-populate graph
		for i := 0; i < 20; i++ {
			from := &Unit{Name: fmt.Sprintf("dot-unit-%d", i), Status: StatusPending}
			to := &Unit{Name: fmt.Sprintf("dot-unit-%d", i+1), Status: StatusPending}
			err := graph.AddEdge(from, to, StatusCompleted)
			require.NoError(t, err)
		}

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
				from := &Unit{Name: fmt.Sprintf("writer-dot-%d", writerID), Status: StatusPending}
				to := &Unit{Name: fmt.Sprintf("writer-dot-target-%d", writerID), Status: StatusPending}
				graph.AddEdge(from, to, StatusCompleted)
			}(i)
		}

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

	t.Run("StressTest", func(t *testing.T) {
		t.Parallel()

		graph := &unit.Graph[Status, *Unit]{}
		var wg sync.WaitGroup
		const numGoroutines = 200
		operations := make([]string, 0, 1000)
		var mu sync.Mutex

		// Launch goroutines performing random operations
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				start := time.Now()
				operationCount := 0

				for time.Since(start) < 500*time.Millisecond && operationCount < 50 {
					operation := float32(secureRandInt(100)) / 100.0

					if operation < 0.6 { // 60% reads
						// Read operation
						testUnit := &Unit{Name: fmt.Sprintf("stress-read-%d", goroutineID), Status: StatusPending}
						forwardEdges := graph.GetForwardAdjacentVertices(testUnit)
						reverseEdges := graph.GetReverseAdjacentVertices(testUnit)

						// Just verify no panics (results may be nil for non-existent vertices)
						_ = forwardEdges
						_ = reverseEdges

						mu.Lock()
						operations = append(operations, "read")
						mu.Unlock()
					} else { // 40% writes
						// Write operation
						from := &Unit{Name: fmt.Sprintf("stress-write-%d-%d", goroutineID, operationCount), Status: StatusPending}
						to := &Unit{Name: fmt.Sprintf("stress-write-target-%d-%d", goroutineID, operationCount), Status: StatusPending}
						graph.AddEdge(from, to, StatusCompleted)

						mu.Lock()
						operations = append(operations, "write")
						mu.Unlock()
					}

					operationCount++
					time.Sleep(time.Microsecond) // Small delay
				}
			}(i)
		}

		wg.Wait()

		// Verify we performed operations
		require.Greater(t, len(operations), 0)

		// Save DOT file for analysis
		saveDOTFile(t, graph, "stress-test-final")

		// Verify graph is still functional
		dot, err := graph.ToDOT("stress-test")
		require.NoError(t, err)
		require.NotEmpty(t, dot)
	})
}
