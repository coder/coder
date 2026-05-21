package unit

import (
	"fmt"
	"sync"

	"golang.org/x/xerrors"
	"gonum.org/v1/gonum/graph/encoding/dot"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"
)

// Graph provides a bidirectional interface over gonum's directed graph implementation.
// While the underlying gonum graph is directed, we overlay bidirectional semantics
// by distinguishing between forward and reverse edges. Wanting and being wanted by
// other units are related but different concepts that have different graph traversal
// implications when Units update their status.
//
// The graph stores edge types to represent different relationships between units,
// allowing for domain-specific semantics beyond simple connectivity.
type Graph[EdgeType, VertexType comparable] struct {
	mu sync.RWMutex
	// The underlying gonum graph. It stores vertices and edges without knowing about the types of the vertices and edges.
	gonumGraph *simple.DirectedGraph
	// Maps vertices to their IDs so that a gonum vertex ID can be used to lookup the vertex type.
	vertexToID map[VertexType]int64
	// Maps vertex IDs to their types so that a vertex type can be used to lookup the gonum vertex ID.
	idToVertex map[int64]VertexType
	// The next ID to assign to a vertex.
	nextID int64
	// Store edge types by "fromID->toID" key. This is used to lookup the edge type for a given edge.
	edgeTypes map[string]EdgeType
}

// Edge is a convenience type for representing an edge in the graph.
// It encapsulates the from and to vertices and the edge type itself.
type Edge[EdgeType, VertexType comparable] struct {
	From VertexType
	To   VertexType
	Edge EdgeType
}

// AddEdge adds an edge to the graph. It initializes the graph and metadata on first use,
// checks for cycles, and adds the edge to the gonum graph.
func (g *Graph[EdgeType, VertexType]) AddEdge(from, to VertexType, edge EdgeType) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.gonumGraph == nil {
		g.gonumGraph = simple.NewDirectedGraph()
		g.vertexToID = make(map[VertexType]int64)
		g.idToVertex = make(map[int64]VertexType)
		g.edgeTypes = make(map[string]EdgeType)
		g.nextID = 1
	}

	fromID := g.getOrCreateVertexID(from)
	toID := g.getOrCreateVertexID(to)

	if g.canReach(to, from) {
		return xerrors.Errorf("adding edge (%v -> %v): %w", from, to, ErrCycleDetected)
	}

	g.gonumGraph.SetEdge(simple.Edge{F: simple.Node(fromID), T: simple.Node(toID)})

	edgeKey := fmt.Sprintf("%d->%d", fromID, toID)
	g.edgeTypes[edgeKey] = edge

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
	toNodes := g.gonumGraph.From(fromID)
	for toNodes.Next() {
		toID := toNodes.Node().ID()
		to := g.idToVertex[toID]

		// Get the edge type
		edgeKey := fmt.Sprintf("%d->%d", fromID, toID)
		edgeType := g.edgeTypes[edgeKey]

		edges = append(edges, Edge[EdgeType, VertexType]{From: from, To: to, Edge: edgeType})
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
	fromNodes := g.gonumGraph.To(toID)
	for fromNodes.Next() {
		fromID := fromNodes.Node().ID()
		from := g.idToVertex[fromID]

		// Get the edge type
		edgeKey := fmt.Sprintf("%d->%d", fromID, toID)
		edgeType := g.edgeTypes[edgeKey]

		edges = append(edges, Edge[EdgeType, VertexType]{From: from, To: to, Edge: edgeType})
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

	// Add the node to the gonum graph
	g.gonumGraph.AddNode(simple.Node(id))

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

	// Use gonum's built-in path existence check
	return topo.PathExistsIn(g.gonumGraph, simple.Node(startID), simple.Node(endID))
}

// ToDOT exports the graph to DOT format for visualization
func (g *Graph[EdgeType, VertexType]) ToDOT(name string) (string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.gonumGraph == nil {
		return "", xerrors.New("graph is not initialized")
	}

	// Marshal the graph to DOT format
	dotBytes, err := dot.Marshal(g.gonumGraph, name, "", "  ")
	if err != nil {
		return "", xerrors.Errorf("failed to marshal graph to DOT: %w", err)
	}

	return string(dotBytes), nil
}
