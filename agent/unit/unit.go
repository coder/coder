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

type UnitGraph struct {
	*Graph[Status, *Unit]
}

func (g *UnitGraph) AddEdge(from, to *Unit, edge Status) {
	g.Graph.AddEdge(from, to, edge)
}

type UnitEdge = Edge[Status, *Unit]

func NewUnitGraph() *UnitGraph {
	return &UnitGraph{Graph: NewGraph[Status, *Unit]()}
}
