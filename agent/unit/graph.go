package unit

import (
	"fmt"
	"sync"

	"golang.org/x/xerrors"
	"gonum.org/v1/gonum/graph/encoding/dot"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"
)

// Graph is an bidirectional adjacency list representation of a graph.
// It is considered bidirectional instead of undirected, because we distinguish
// between forward and reverse edges. Wanting and being wanted by other units
// are related but different concepts that have different graph traversal implications
// when Units update their status. Adding one of these directions necessarily adds
// the other to the complementary unit.
//
// Graph vertices often have their own attributes specific to the problem domain.
// In this case we need to distinguish between different edge types to represent
// the different relationships between units.
type Graph[EdgeType, VertexType comparable] struct {
	mu         sync.RWMutex
	gonumGraph *simple.DirectedGraph
	vertexToID map[VertexType]int64
	idToVertex map[int64]VertexType
	nextID     int64
	edgeTypes  map[string]EdgeType // Store edge types by "fromID->toID" key
}

// Edge is a convenience type for representing an edge in the graph.
// It encapsulates the from and to vertices and the edge type itself.
type Edge[EdgeType, VertexType comparable] struct {
	From VertexType
	To   VertexType
	Edge EdgeType
}

// AddEdge adds an edge to the graph. It initializes the graph if it doesn't exist
// and adds the edge to the gonum graph.
func (g *Graph[EdgeType, VertexType]) AddEdge(from, to VertexType, edge EdgeType) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Initialize the graph if it doesn't exist
	if g.gonumGraph == nil {
		g.gonumGraph = simple.NewDirectedGraph()
		g.vertexToID = make(map[VertexType]int64)
		g.idToVertex = make(map[int64]VertexType)
		g.edgeTypes = make(map[string]EdgeType)
		g.nextID = 1
	}

	// Get or create IDs for vertices
	fromID := g.getOrCreateVertexID(from)
	toID := g.getOrCreateVertexID(to)

	if g.canReach(to, from) {
		return xerrors.Errorf("adding edge (%v -> %v) would create a cycle", from, to)
	}

	// Add the edge to the gonum graph
	g.gonumGraph.SetEdge(simple.Edge{F: simple.Node(fromID), T: simple.Node(toID)})

	// Store the edge type
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

// getOrCreateVertexID returns the ID for a vertex, creating it if it doesn't exist
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
