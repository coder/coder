package unit

import "sync"

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
	mu                   sync.RWMutex
	adjacencyList        map[VertexType]map[VertexType]EdgeType
	reverseAdjacencyList map[VertexType]map[VertexType]EdgeType
}

// Edge is a convenience type for representing an edge in the graph.
// It encapsulates the from and to vertices and the edge type itself.
type Edge[EdgeType, VertexType comparable] struct {
	From VertexType
	To   VertexType
	Edge EdgeType
}

func NewGraph[EdgeType, VertexType comparable]() *Graph[EdgeType, VertexType] {
	return &Graph[EdgeType, VertexType]{
		adjacencyList:        make(map[VertexType]map[VertexType]EdgeType),
		reverseAdjacencyList: make(map[VertexType]map[VertexType]EdgeType),
	}
}

// AddEdge adds an edge to the graph. It initializes the adjacency lists if they don't exist
// and adds the edge to both the adjacency list and the reverse adjacency list.
func (g *Graph[EdgeType, VertexType]) AddEdge(from, to VertexType, edge EdgeType) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Initialize the adjacency lists if they don't exist
	if _, ok := g.adjacencyList[from]; !ok {
		g.adjacencyList[from] = make(map[VertexType]EdgeType)
	}
	if _, ok := g.reverseAdjacencyList[to]; !ok {
		g.reverseAdjacencyList[to] = make(map[VertexType]EdgeType)
	}

	// Add the edge to the adjacency lists
	g.adjacencyList[from][to] = edge
	g.reverseAdjacencyList[to][from] = edge
}

// GetAdjacentVertices returns all the edges that originate from the given vertex.
func (g *Graph[EdgeType, VertexType]) GetAdjacentVertices(from VertexType) []Edge[EdgeType, VertexType] {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Get the edges from the adjacency list
	edges := make([]Edge[EdgeType, VertexType], 0, len(g.adjacencyList[from]))
	for to, edge := range g.adjacencyList[from] {
		edges = append(edges, Edge[EdgeType, VertexType]{From: from, To: to, Edge: edge})
	}

	return edges
}

// GetReverseAdjacentVertices returns all the edges that terminate at the given vertex.
func (g *Graph[EdgeType, VertexType]) GetReverseAdjacentVertices(to VertexType) []Edge[EdgeType, VertexType] {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Get the edges from the reverse adjacency list
	edges := make([]Edge[EdgeType, VertexType], 0, len(g.reverseAdjacencyList[to]))
	for from, edge := range g.reverseAdjacencyList[to] {
		edges = append(edges, Edge[EdgeType, VertexType]{From: from, To: to, Edge: edge})
	}

	return edges
}
