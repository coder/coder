package unit

import (
	"sync"

	"golang.org/x/xerrors"
)

// ErrConsumerNotFound is returned when a consumer ID is not registered.
var ErrConsumerNotFound = xerrors.New("consumer not found")

// ErrCannotUpdateOtherConsumer is returned when attempting to update another consumer's status.
var ErrCannotUpdateOtherConsumer = xerrors.New("cannot update other consumer's status")

// dependencyVertex represents a vertex in the dependency graph that is associated with a consumer.
type dependencyVertex[ConsumerID comparable] struct {
	ID ConsumerID
}

// Dependency represents a dependency relationship between consumers.
type Dependency[StatusType, ConsumerID comparable] struct {
	Consumer       ConsumerID
	DependsOn      ConsumerID
	RequiredStatus StatusType
	CurrentStatus  StatusType
	IsSatisfied    bool
}

// DependencyTracker provides reactive dependency tracking over a Graph.
// It manages consumer registration, dependency relationships, and status updates
// with automatic recalculation of readiness when dependencies are satisfied.
type DependencyTracker[StatusType, ConsumerID comparable] struct {
	mu sync.RWMutex

	// The underlying graph that stores dependency relationships
	graph *Graph[StatusType, *dependencyVertex[ConsumerID]]

	// Track current status of each consumer
	consumerStatus map[ConsumerID]StatusType

	// Track readiness state (cached to avoid repeated graph traversal)
	consumerReadiness map[ConsumerID]bool

	// Track which consumers are registered
	registeredConsumers map[ConsumerID]bool

	// Store vertex instances for each consumer to ensure consistent references
	consumerVertices map[ConsumerID]*dependencyVertex[ConsumerID]
}

// NewDependencyTracker creates a new DependencyTracker instance.
func NewDependencyTracker[StatusType, ConsumerID comparable]() *DependencyTracker[StatusType, ConsumerID] {
	return &DependencyTracker[StatusType, ConsumerID]{
		graph:               &Graph[StatusType, *dependencyVertex[ConsumerID]]{},
		consumerStatus:      make(map[ConsumerID]StatusType),
		consumerReadiness:   make(map[ConsumerID]bool),
		registeredConsumers: make(map[ConsumerID]bool),
		consumerVertices:    make(map[ConsumerID]*dependencyVertex[ConsumerID]),
	}
}

// Register registers a new consumer as a vertex in the dependency graph.
func (dt *DependencyTracker[StatusType, ConsumerID]) Register(id ConsumerID) error {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	if dt.registeredConsumers[id] {
		return xerrors.Errorf("consumer %v is already registered", id)
	}

	// Create and store the vertex for this consumer
	vertex := &dependencyVertex[ConsumerID]{ID: id}
	dt.consumerVertices[id] = vertex
	dt.registeredConsumers[id] = true
	dt.consumerReadiness[id] = true // New consumers start as ready (no dependencies)

	return nil
}

// AddDependency adds a dependency relationship between consumers.
// The consumer depends on the dependsOn consumer reaching the requiredStatus.
func (dt *DependencyTracker[StatusType, ConsumerID]) AddDependency(consumer ConsumerID, dependsOn ConsumerID, requiredStatus StatusType) error {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	if !dt.registeredConsumers[consumer] {
		return xerrors.Errorf("consumer %v is not registered", consumer)
	}
	if !dt.registeredConsumers[dependsOn] {
		return xerrors.Errorf("consumer %v is not registered", dependsOn)
	}

	// Get the stored vertices for both consumers
	consumerVertex := dt.consumerVertices[consumer]
	dependsOnVertex := dt.consumerVertices[dependsOn]

	// Add the dependency edge to the graph
	// The edge goes from consumer to dependsOn, representing the dependency
	err := dt.graph.AddEdge(consumerVertex, dependsOnVertex, requiredStatus)
	if err != nil {
		return xerrors.Errorf("failed to add dependency: %w", err)
	}

	// Recalculate readiness for the consumer since it now has a dependency
	dt.recalculateReadinessUnsafe(consumer)

	return nil
}

// UpdateStatus updates a consumer's status and recalculates readiness for affected dependents.
func (dt *DependencyTracker[StatusType, ConsumerID]) UpdateStatus(consumer ConsumerID, newStatus StatusType) error {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	if !dt.registeredConsumers[consumer] {
		return ErrConsumerNotFound
	}

	// Update the consumer's status
	dt.consumerStatus[consumer] = newStatus

	// Get all consumers that depend on this one (reverse adjacent vertices)
	consumerVertex := dt.consumerVertices[consumer]
	dependentEdges := dt.graph.GetReverseAdjacentVertices(consumerVertex)

	// Recalculate readiness for all dependents
	for _, edge := range dependentEdges {
		dt.recalculateReadinessUnsafe(edge.From.ID)
	}

	return nil
}

// IsReady checks if all dependencies for a consumer are satisfied.
func (dt *DependencyTracker[StatusType, ConsumerID]) IsReady(consumer ConsumerID) (bool, error) {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	if !dt.registeredConsumers[consumer] {
		return false, ErrConsumerNotFound
	}

	return dt.consumerReadiness[consumer], nil
}

// GetUnmetDependencies returns a list of unsatisfied dependencies for a consumer.
func (dt *DependencyTracker[StatusType, ConsumerID]) GetUnmetDependencies(consumer ConsumerID) ([]Dependency[StatusType, ConsumerID], error) {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	if !dt.registeredConsumers[consumer] {
		return nil, ErrConsumerNotFound
	}

	consumerVertex := dt.consumerVertices[consumer]
	forwardEdges := dt.graph.GetForwardAdjacentVertices(consumerVertex)

	var unmetDependencies []Dependency[StatusType, ConsumerID]

	for _, edge := range forwardEdges {
		dependsOnConsumer := edge.To.ID
		requiredStatus := edge.Edge
		currentStatus, exists := dt.consumerStatus[dependsOnConsumer]
		if !exists {
			// If the dependency consumer has no status, it's not satisfied
			var zeroStatus StatusType
			unmetDependencies = append(unmetDependencies, Dependency[StatusType, ConsumerID]{
				Consumer:       consumer,
				DependsOn:      dependsOnConsumer,
				RequiredStatus: requiredStatus,
				CurrentStatus:  zeroStatus, // Zero value
				IsSatisfied:    false,
			})
		} else {
			isSatisfied := currentStatus == requiredStatus
			if !isSatisfied {
				unmetDependencies = append(unmetDependencies, Dependency[StatusType, ConsumerID]{
					Consumer:       consumer,
					DependsOn:      dependsOnConsumer,
					RequiredStatus: requiredStatus,
					CurrentStatus:  currentStatus,
					IsSatisfied:    false,
				})
			}
		}
	}

	return unmetDependencies, nil
}

// recalculateReadinessUnsafe recalculates the readiness state for a consumer.
// This method assumes the caller holds the write lock.
func (dt *DependencyTracker[StatusType, ConsumerID]) recalculateReadinessUnsafe(consumer ConsumerID) {
	consumerVertex := dt.consumerVertices[consumer]
	forwardEdges := dt.graph.GetForwardAdjacentVertices(consumerVertex)

	// If there are no dependencies, the consumer is ready
	if len(forwardEdges) == 0 {
		dt.consumerReadiness[consumer] = true
		return
	}

	// Check if all dependencies are satisfied
	allSatisfied := true
	for _, edge := range forwardEdges {
		dependsOnConsumer := edge.To.ID
		requiredStatus := edge.Edge
		currentStatus, exists := dt.consumerStatus[dependsOnConsumer]
		if !exists || currentStatus != requiredStatus {
			allSatisfied = false
			break
		}
	}

	dt.consumerReadiness[consumer] = allSatisfied
}

// GetGraph returns the underlying graph for visualization and debugging.
// This should be used carefully as it exposes the internal graph structure.
func (dt *DependencyTracker[StatusType, ConsumerID]) GetGraph() *Graph[StatusType, *dependencyVertex[ConsumerID]] {
	return dt.graph
}

// ExportDOT exports the dependency graph to DOT format for visualization.
func (dt *DependencyTracker[StatusType, ConsumerID]) ExportDOT(name string) (string, error) {
	return dt.graph.ToDOT(name)
}
