package unit

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"golang.org/x/xerrors"
)

// Graph is a directed acyclic graph with typed edges. Forward and reverse
// adjacency are both indexed. Wanting and being wanted by other units are
// related but different concepts that have different graph traversal
// implications when Units update their status.
//
// The graph stores edge types to represent different relationships between units,
// allowing for domain-specific semantics beyond simple connectivity.
type Graph[EdgeType, VertexType comparable] struct {
	mu sync.RWMutex
	// Maps vertices to their IDs. IDs are assigned in insertion order and
	// give DOT output a stable ordering.
	vertexToID map[VertexType]int64
	// Maps vertex IDs back to their vertices.
	idToVertex map[int64]VertexType
	// The next ID to assign to a vertex.
	nextID int64
	// forward maps a vertex ID to the IDs it has edges to, with edge types.
	forward map[int64]map[int64]EdgeType
	// reverse maps a vertex ID to the IDs that have edges to it.
	reverse map[int64]map[int64]struct{}
}

// Edge is a convenience type for representing an edge in the graph.
// It encapsulates the from and to vertices and the edge type itself.
type Edge[EdgeType, VertexType comparable] struct {
	From VertexType
	To   VertexType
	Edge EdgeType
}

// AddEdge adds an edge to the graph. It initializes the graph on first use and
// rejects edges that would create a cycle.
func (g *Graph[EdgeType, VertexType]) AddEdge(from, to VertexType, edge EdgeType) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.vertexToID == nil {
		g.vertexToID = make(map[VertexType]int64)
		g.idToVertex = make(map[int64]VertexType)
		g.forward = make(map[int64]map[int64]EdgeType)
		g.reverse = make(map[int64]map[int64]struct{})
		g.nextID = 1
	}

	fromID := g.getOrCreateVertexID(from)
	toID := g.getOrCreateVertexID(to)

	if g.canReach(to, from) {
		return xerrors.Errorf("adding edge (%v -> %v): %w", from, to, ErrCycleDetected)
	}

	g.forward[fromID][toID] = edge
	g.reverse[toID][fromID] = struct{}{}

	return nil
}

// GetForwardAdjacentVertices returns all the edges that originate from the given vertex.
func (g *Graph[EdgeType, VertexType]) GetForwardAdjacentVertices(from VertexType) []Edge[EdgeType, VertexType] {
	g.mu.RLock()
	defer g.mu.RUnlock()

	fromID, exists := g.vertexToID[from]
	if !exists {
		return []Edge[EdgeType, VertexType]{}
	}

	edges := []Edge[EdgeType, VertexType]{}
	for toID, edgeType := range g.forward[fromID] {
		edges = append(edges, Edge[EdgeType, VertexType]{From: from, To: g.idToVertex[toID], Edge: edgeType})
	}

	return edges
}

// GetReverseAdjacentVertices returns all the edges that terminate at the given vertex.
func (g *Graph[EdgeType, VertexType]) GetReverseAdjacentVertices(to VertexType) []Edge[EdgeType, VertexType] {
	g.mu.RLock()
	defer g.mu.RUnlock()

	toID, exists := g.vertexToID[to]
	if !exists {
		return []Edge[EdgeType, VertexType]{}
	}

	edges := []Edge[EdgeType, VertexType]{}
	for fromID := range g.reverse[toID] {
		edgeType := g.forward[fromID][toID]
		edges = append(edges, Edge[EdgeType, VertexType]{From: g.idToVertex[fromID], To: to, Edge: edgeType})
	}

	return edges
}

// getOrCreateVertexID returns the ID for a vertex, creating it if it doesn't exist.
func (g *Graph[EdgeType, VertexType]) getOrCreateVertexID(vertex VertexType) int64 {
	if id, exists := g.vertexToID[vertex]; exists {
		return id
	}

	id := g.nextID
	g.nextID++
	g.vertexToID[vertex] = id
	g.idToVertex[id] = vertex
	g.forward[id] = make(map[int64]EdgeType)
	g.reverse[id] = make(map[int64]struct{})

	return id
}

// canReach checks if there is a path from the start vertex to the end vertex.
func (g *Graph[EdgeType, VertexType]) canReach(start, end VertexType) bool {
	if start == end {
		return true
	}

	startID, startExists := g.vertexToID[start]
	endID, endExists := g.vertexToID[end]

	if !startExists || !endExists {
		return false
	}

	visited := map[int64]struct{}{startID: {}}
	stack := []int64{startID}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for next := range g.forward[id] {
			if next == endID {
				return true
			}
			if _, seen := visited[next]; seen {
				continue
			}
			visited[next] = struct{}{}
			stack = append(stack, next)
		}
	}
	return false
}

// ToDOT exports the graph to DOT format for visualization. Vertices are
// written by their numeric IDs, and nodes and edges are sorted by ID.
func (g *Graph[EdgeType, VertexType]) ToDOT(name string) (string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.vertexToID == nil {
		return "", xerrors.New("graph is not initialized")
	}

	ids := slices.Sorted(maps.Keys(g.idToVertex))
	var edgeLines []string
	for _, from := range ids {
		for _, to := range slices.Sorted(maps.Keys(g.forward[from])) {
			edgeLines = append(edgeLines, fmt.Sprintf("  %d -> %d;", from, to))
		}
	}

	header := "strict digraph {"
	if name != "" {
		header = "strict digraph " + dotID(name) + " {"
	}
	lines := []string{header, "  // Node definitions."}
	for _, id := range ids {
		lines = append(lines, fmt.Sprintf("  %d;", id))
	}
	if len(edgeLines) > 0 {
		lines = append(lines, "", "  // Edge definitions.")
		lines = append(lines, edgeLines...)
	}
	lines = append(lines, "}")

	return strings.Join(lines, "\n"), nil
}

// dotID returns s as a DOT identifier, quoting it unless it is a plain
// alphanumeric identifier that is not a DOT keyword.
func dotID(s string) string {
	switch strings.ToLower(s) {
	case "node", "edge", "graph", "digraph", "subgraph", "strict":
		return fmt.Sprintf("%q", s)
	}
	for i, r := range s {
		isLetter := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		if !isLetter && (!isDigit || i == 0) {
			return fmt.Sprintf("%q", s)
		}
	}
	return s
}
