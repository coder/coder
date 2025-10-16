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

// DependencyCoordinator is a composition of multiple structures that work together to
// coordinate the dependencies between units. It is composed of a Graph of Units and Statuses
// to track propagation of status changes through the graph. It also provides a pub-sub mechanism
// to notify units when their dependencies are updated such that they can update their own status
// accordingly.
type DependencyCoordinator struct {
	*Graph[Status, *Unit]
	// TODO(sas): implement pub-sub mechanism to notify units when their dependencies are updated
}

func (g *DependencyCoordinator) AddEdge(from, to *Unit, edge Status) {
	g.Graph.AddEdge(from, to, edge)
}

type DependencyEdge = Edge[Status, *Unit]

func NewDependencyCoordinator() *DependencyCoordinator {
	return &DependencyCoordinator{Graph: &Graph[Status, *Unit]{}}
}
