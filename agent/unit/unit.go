package unit

type Unit struct {
	Name   string
	Status Status
}

type Status string

const (
	StatusPending   Status = "pending"
	StatusStarted   Status = "started"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type DependencyCoordinator struct {
	*Graph[Status, *Unit]
}

func (g *DependencyCoordinator) AddEdge(from, to *Unit, edge Status) {
	g.Graph.AddEdge(from, to, edge)
}

type DependencyEdge = Edge[Status, *Unit]

func NewDependencyCoordinator() *DependencyCoordinator {
	return &DependencyCoordinator{Graph: NewGraph[Status, *Unit]()}
}
