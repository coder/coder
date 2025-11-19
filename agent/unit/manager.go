package unit

import (
	"sync"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/util/slice"
)

var (
	ErrUnitNotFound             = "unit not found"
	ErrUnitAlreadyRegistered    = "unit already registered"
	ErrCannotUpdateOtherUnit    = "cannot update other unit's status"
	ErrDependenciesNotSatisfied = "unit dependencies not satisfied"
	ErrSameStatusAlreadySet     = "same status already set"
	ErrCycleDetected            = "cycle detected"
)

// Status represents the status of a unit.
type Status string

// Status constants for dependency tracking
const (
	StatusNotRegistered Status = ""
	StatusPending       Status = "pending"
	StatusStarted       Status = "started"
	StatusComplete      Status = "completed"
)

// ID provides a type narrowed representation of the unique identifier of a unit.
type ID string

// Unit represents a point-in-time snapshot of a vertex in the dependency graph.
// Units may depend on other units, or be depended on by other units. The unit struct
// is not aware of updates made to the dependency graph after it is initialized and should
// not be cached.
type Unit struct {
	id     ID
	status Status
	// ready is true if all dependencies are satisfied.
	// It does not have an accessor method on Unit, because a unit cannot know whether it is ready.
	// Only the Manager can calculate whether a unit is ready based on knowledge of the dependency graph.
	// To discourage use of an outdated readiness value, only the Manager should set and return this field.
	ready bool
}

func (u *Unit) ID() ID {
	return u.id
}

func (u *Unit) Status() Status {
	return u.status
}

// Dependency represents a dependency relationship between units.
type Dependency struct {
	Unit           ID
	DependsOn      ID
	RequiredStatus Status
	CurrentStatus  Status
	IsSatisfied    bool
}

// Manager provides reactive dependency tracking over a Graph.
// It manages Unit registration, dependency relationships, and status updates
// with automatic recalculation of readiness when dependencies are satisfied.
type Manager struct {
	mu sync.RWMutex

	// The underlying graph that stores dependency relationships
	graph *Graph[Status, ID]

	// Store vertex instances for each unit to ensure consistent references
	units map[ID]Unit
}

// NewManager creates a new Manager instance.
func NewManager() *Manager {
	return &Manager{
		graph: &Graph[Status, ID]{},
		units: make(map[ID]Unit),
	}
}

// Register adds a unit to the manager if it is not already registered.
// If a Unit is already registered (per the ID field), it is not updated.
func (m *Manager) Register(id ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.registered(id) {
		return xerrors.Errorf(ErrUnitAlreadyRegistered)
	}

	m.units[id] = Unit{
		id:     id,
		status: StatusPending,
		ready:  true,
	}

	return nil
}

// registered checks if a unit is registered in the manager.
func (m *Manager) registered(id ID) bool {
	return m.units[id].status != StatusNotRegistered
}

// Unit fetches a unit from the manager. If the unit does not exist,
// it returns the Unit zero-value as a placeholder unit, because
// units may depend on other units that have not yet been created.
func (m *Manager) Unit(id ID) Unit {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.units[id]
}

func (m *Manager) IsReady(id ID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.registered(id) {
		return false
	}

	return m.units[id].ready
}

// AddDependency adds a dependency relationship between units.
// The unit depends on the dependsOn unit reaching the requiredStatus.
func (m *Manager) AddDependency(unit ID, dependsOn ID, requiredStatus Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.registered(unit) {
		return xerrors.Errorf("unit %v is not registered: %w", unit, ErrUnitNotFound)
	}

	// Add the dependency edge to the graph
	// The edge goes from unit to dependsOn, representing the dependency
	err := m.graph.AddEdge(unit, dependsOn, requiredStatus)
	if err != nil {
		return xerrors.Errorf("failed to add dependency: %w", err)
	}

	// Recalculate readiness for the unit since it now has a new dependency
	m.recalculateReadinessUnsafe(unit)

	return nil
}

// UpdateStatus updates a unit's status and recalculates readiness for affected dependents.
func (m *Manager) UpdateStatus(unit ID, newStatus Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	u := m.units[unit]
	if !m.registered(unit) {
		return xerrors.Errorf("unit %v is not registered: %w", unit, ErrUnitNotFound)
	}
	if u.status == newStatus {
		return xerrors.Errorf("%s", ErrSameStatusAlreadySet)
	}

	u.status = newStatus
	m.units[unit] = u

	// Get all units that depend on this one (reverse adjacent vertices)
	dependentEdges := m.graph.GetReverseAdjacentVertices(unit)

	// Recalculate readiness for all dependents
	for _, edge := range dependentEdges {
		m.recalculateReadinessUnsafe(edge.From)
	}

	return nil
}

// GetUnmetDependencies returns a list of unsatisfied dependencies for a unit.
func (m *Manager) GetUnmetDependencies(unit ID) ([]Dependency, error) {
	allDependencies, err := m.GetAllDependencies(unit)
	if err != nil {
		return nil, err
	}

	var unmetDependencies []Dependency = slice.Filter(allDependencies, func(dependency Dependency) bool {
		return !dependency.IsSatisfied
	})

	return unmetDependencies, nil
}

// recalculateReadinessUnsafe recalculates the readiness state for a unit.
// This method assumes the caller holds the write lock.
func (m *Manager) recalculateReadinessUnsafe(unit ID) {
	u := m.units[unit]
	dependencies := m.graph.GetForwardAdjacentVertices(unit)

	if len(dependencies) == 0 {
		u.ready = true
		m.units[unit] = u
		return
	}

	allSatisfied := true
	for _, dependency := range dependencies {
		requiredStatus := dependency.Edge
		dependsOnUnit := m.units[dependency.To]
		if dependsOnUnit.status != requiredStatus {
			allSatisfied = false
			break
		}
	}

	u.ready = allSatisfied
	m.units[unit] = u
}

// GetGraph returns the underlying graph for visualization and debugging.
// This should be used carefully as it exposes the internal graph structure.
func (m *Manager) GetGraph() *Graph[Status, ID] {
	return m.graph
}

// GetStatus returns the current status of a unit.
func (m *Manager) GetStatus(unit ID) Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	u := m.units[unit]
	return u.status
}

// GetAllDependencies returns all dependencies for a unit, both satisfied and unsatisfied.
func (m *Manager) GetAllDependencies(unit ID) ([]Dependency, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.registered(unit) {
		return nil, xerrors.Errorf("%s", ErrUnitNotFound)
	}

	forwardEdges := m.graph.GetForwardAdjacentVertices(unit)

	var allDependencies []Dependency

	for _, edge := range forwardEdges {
		dependsOnUnit := m.units[edge.To]
		requiredStatus := edge.Edge
		allDependencies = append(allDependencies, Dependency{
			Unit:           unit,
			DependsOn:      dependsOnUnit.id,
			RequiredStatus: requiredStatus,
			CurrentStatus:  dependsOnUnit.status,
			IsSatisfied:    dependsOnUnit.status == requiredStatus,
		})
	}

	return allDependencies, nil
}

// ExportDOT exports the dependency graph to DOT format for visualization.
func (m *Manager) ExportDOT(name string) (string, error) {
	return m.graph.ToDOT(name)
}
