package unit

import "sync"

// Graph is an bidirectional adjacency list representation of a graph.
type Graph[EdgeType, VertexType comparable] struct {
	mu                   sync.RWMutex
	adjacencyList        map[VertexType]map[VertexType]EdgeType
	reverseAdjacencyList map[VertexType]map[VertexType]EdgeType
}

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
