package unit

import (
	"sync"

	"golang.org/x/xerrors"
)

var (
	// ErrUnitNotFound is returned when a unit ID is not registered.
	ErrUnitNotFound = xerrors.New("unit not found")

	// ErrUnitAlreadyRegistered is returned when a unit ID is already registered.
	ErrUnitAlreadyRegistered = xerrors.New("unit already registered")

	// ErrCannotUpdateOtherUnit is returned when attempting to update another unit's status.
	ErrCannotUpdateOtherUnit = xerrors.New("cannot update other unit's status")

	// ErrDependenciesNotSatisfied is returned when a unit's dependencies are not satisfied.
	ErrDependenciesNotSatisfied = xerrors.New("unit dependencies not satisfied")

	// ErrSameStatusAlreadySet is returned when attempting to set the same status as the current status.
	ErrSameStatusAlreadySet = xerrors.New("same status already set")
)

// Status constants for dependency tracking
const (
	StatusStarted  = "started"
	StatusComplete = "completed"
)

// dependencyVertex represents a vertex in the dependency graph that is associated with a unit.
type dependencyVertex[UnitID comparable] struct {
	ID UnitID
}

// Dependency represents a dependency relationship between units.
type Dependency[StatusType, UnitID comparable] struct {
	Unit           UnitID
	DependsOn      UnitID
	RequiredStatus StatusType
	CurrentStatus  StatusType
	IsSatisfied    bool
}

// Manager provides reactive dependency tracking over a Graph.
// It manages unit registration, dependency relationships, and status updates
// with automatic recalculation of readiness when dependencies are satisfied.
type Manager[StatusType, UnitID comparable] struct {
	mu sync.RWMutex

	// The underlying graph that stores dependency relationships
	graph *Graph[StatusType, *dependencyVertex[UnitID]]

	// Track current status of each unit
	unitStatus map[UnitID]StatusType

	// Track readiness state (cached to avoid repeated graph traversal)
	unitReadiness map[UnitID]bool

	// Track which units are registered
	registeredUnits map[UnitID]bool

	// Store vertex instances for each unit to ensure consistent references
	unitVertices map[UnitID]*dependencyVertex[UnitID]
}

// NewManager creates a new Manager instance.
func NewManager[StatusType, UnitID comparable]() *Manager[StatusType, UnitID] {
	return &Manager[StatusType, UnitID]{
		graph:           &Graph[StatusType, *dependencyVertex[UnitID]]{},
		unitStatus:      make(map[UnitID]StatusType),
		unitReadiness:   make(map[UnitID]bool),
		registeredUnits: make(map[UnitID]bool),
		unitVertices:    make(map[UnitID]*dependencyVertex[UnitID]),
	}
}

// Register registers a new unit as a vertex in the dependency graph.
func (dt *Manager[StatusType, UnitID]) Register(id UnitID) error {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	if dt.registeredUnits[id] {
		return ErrUnitAlreadyRegistered
	}

	// Create and store the vertex for this unit
	vertex := &dependencyVertex[UnitID]{ID: id}
	dt.unitVertices[id] = vertex
	dt.registeredUnits[id] = true
	dt.unitReadiness[id] = true // New units start as ready (no dependencies)

	return nil
}

// AddDependency adds a dependency relationship between units.
// The unit depends on the dependsOn unit reaching the requiredStatus.
func (dt *Manager[StatusType, UnitID]) AddDependency(unit UnitID, dependsOn UnitID, requiredStatus StatusType) error {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	if !dt.registeredUnits[unit] {
		return xerrors.Errorf("unit %v is not registered", unit)
	}
	if !dt.registeredUnits[dependsOn] {
		return xerrors.Errorf("unit %v is not registered", dependsOn)
	}

	// Get the stored vertices for both units
	unitVertex := dt.unitVertices[unit]
	dependsOnVertex := dt.unitVertices[dependsOn]

	// Add the dependency edge to the graph
	// The edge goes from unit to dependsOn, representing the dependency
	err := dt.graph.AddEdge(unitVertex, dependsOnVertex, requiredStatus)
	if err != nil {
		return xerrors.Errorf("failed to add dependency: %w", err)
	}

	// Recalculate readiness for the unit since it now has a dependency
	dt.recalculateReadinessUnsafe(unit)

	return nil
}

// UpdateStatus updates a unit's status and recalculates readiness for affected dependents.
func (dt *Manager[StatusType, UnitID]) UpdateStatus(unit UnitID, newStatus StatusType) error {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	if !dt.registeredUnits[unit] {
		return ErrUnitNotFound
	}

	// Update the unit's status
	if dt.unitStatus[unit] == newStatus {
		return ErrSameStatusAlreadySet
	}
	dt.unitStatus[unit] = newStatus

	// Get all units that depend on this one (reverse adjacent vertices)
	unitVertex := dt.unitVertices[unit]
	dependentEdges := dt.graph.GetReverseAdjacentVertices(unitVertex)

	// Recalculate readiness for all dependents
	for _, edge := range dependentEdges {
		dt.recalculateReadinessUnsafe(edge.From.ID)
	}

	return nil
}

// IsReady checks if all dependencies for a unit are satisfied.
func (dt *Manager[StatusType, UnitID]) IsReady(unit UnitID) (bool, error) {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	if !dt.registeredUnits[unit] {
		return false, ErrUnitNotFound
	}

	return dt.unitReadiness[unit], nil
}

// GetUnmetDependencies returns a list of unsatisfied dependencies for a unit.
func (dt *Manager[StatusType, UnitID]) GetUnmetDependencies(unit UnitID) ([]Dependency[StatusType, UnitID], error) {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	if !dt.registeredUnits[unit] {
		return nil, ErrUnitNotFound
	}

	unitVertex := dt.unitVertices[unit]
	forwardEdges := dt.graph.GetForwardAdjacentVertices(unitVertex)

	var unmetDependencies []Dependency[StatusType, UnitID]

	for _, edge := range forwardEdges {
		dependsOnUnit := edge.To.ID
		requiredStatus := edge.Edge
		currentStatus, exists := dt.unitStatus[dependsOnUnit]
		if !exists {
			// If the dependency unit has no status, it's not satisfied
			// Zero value represents StatusPending
			var pendingStatus StatusType
			unmetDependencies = append(unmetDependencies, Dependency[StatusType, UnitID]{
				Unit:           unit,
				DependsOn:      dependsOnUnit,
				RequiredStatus: requiredStatus,
				CurrentStatus:  pendingStatus, // StatusPending (zero value)
				IsSatisfied:    false,
			})
		} else {
			isSatisfied := currentStatus == requiredStatus
			if !isSatisfied {
				unmetDependencies = append(unmetDependencies, Dependency[StatusType, UnitID]{
					Unit:           unit,
					DependsOn:      dependsOnUnit,
					RequiredStatus: requiredStatus,
					CurrentStatus:  currentStatus,
					IsSatisfied:    false,
				})
			}
		}
	}

	return unmetDependencies, nil
}

// recalculateReadinessUnsafe recalculates the readiness state for a unit.
// This method assumes the caller holds the write lock.
func (dt *Manager[StatusType, UnitID]) recalculateReadinessUnsafe(unit UnitID) {
	unitVertex := dt.unitVertices[unit]
	forwardEdges := dt.graph.GetForwardAdjacentVertices(unitVertex)

	// If there are no dependencies, the unit is ready
	if len(forwardEdges) == 0 {
		dt.unitReadiness[unit] = true
		return
	}

	// Check if all dependencies are satisfied
	allSatisfied := true
	for _, edge := range forwardEdges {
		dependsOnUnit := edge.To.ID
		requiredStatus := edge.Edge
		currentStatus, exists := dt.unitStatus[dependsOnUnit]
		if !exists || currentStatus != requiredStatus {
			allSatisfied = false
			break
		}
	}

	dt.unitReadiness[unit] = allSatisfied
}

// GetGraph returns the underlying graph for visualization and debugging.
// This should be used carefully as it exposes the internal graph structure.
func (dt *Manager[StatusType, UnitID]) GetGraph() *Graph[StatusType, *dependencyVertex[UnitID]] {
	return dt.graph
}

// GetStatus returns the current status of a unit.
func (dt *Manager[StatusType, UnitID]) GetStatus(unit UnitID) (StatusType, error) {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	if !dt.registeredUnits[unit] {
		// Zero value represents StatusPending
		var pendingStatus StatusType
		return pendingStatus, ErrUnitNotFound
	}

	status, exists := dt.unitStatus[unit]
	if !exists {
		// Zero value represents StatusPending
		var pendingStatus StatusType
		return pendingStatus, nil
	}

	return status, nil
}

// GetAllDependencies returns all dependencies for a unit, both satisfied and unsatisfied.
func (dt *Manager[StatusType, UnitID]) GetAllDependencies(unit UnitID) ([]Dependency[StatusType, UnitID], error) {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	if !dt.registeredUnits[unit] {
		return nil, ErrUnitNotFound
	}

	unitVertex := dt.unitVertices[unit]
	forwardEdges := dt.graph.GetForwardAdjacentVertices(unitVertex)

	var allDependencies []Dependency[StatusType, UnitID]

	for _, edge := range forwardEdges {
		dependsOnUnit := edge.To.ID
		requiredStatus := edge.Edge
		currentStatus, exists := dt.unitStatus[dependsOnUnit]
		if !exists {
			// If the dependency unit has no status, it's not satisfied
			// Zero value represents StatusPending
			var pendingStatus StatusType
			allDependencies = append(allDependencies, Dependency[StatusType, UnitID]{
				Unit:           unit,
				DependsOn:      dependsOnUnit,
				RequiredStatus: requiredStatus,
				CurrentStatus:  pendingStatus, // StatusPending (zero value)
				IsSatisfied:    false,
			})
		} else {
			isSatisfied := currentStatus == requiredStatus
			allDependencies = append(allDependencies, Dependency[StatusType, UnitID]{
				Unit:           unit,
				DependsOn:      dependsOnUnit,
				RequiredStatus: requiredStatus,
				CurrentStatus:  currentStatus,
				IsSatisfied:    isSatisfied,
			})
		}
	}

	return allDependencies, nil
}

// ExportDOT exports the dependency graph to DOT format for visualization.
func (dt *Manager[StatusType, UnitID]) ExportDOT(name string) (string, error) {
	return dt.graph.ToDOT(name)
}
