package unit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/unit"
)

const (
	unitA unit.ID = "serviceA"
	unitB unit.ID = "serviceB"
	unitC unit.ID = "serviceC"
	unitD unit.ID = "serviceD"
)

func TestManager_Register(t *testing.T) {
	t.Parallel()

	t.Run("RegisterNewUnit", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Given: a unit is registered
		err := manager.Register(unitA)
		require.NoError(t, err)

		// Then: the unit should be ready (no dependencies)
		u := manager.Unit(unitA)
		assert.Equal(t, unitA, u.ID())
		assert.Equal(t, unit.StatusPending, u.Status())
		assert.True(t, manager.IsReady(unitA))
	})

	t.Run("RegisterDuplicateUnit", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Given: a unit is registered
		err := manager.Register(unitA)
		require.NoError(t, err)

		// Newly registered units have StatusPending. We update the unit status to StatusStarted,
		// so we can later assert that it is not overwritten back to StatusPending by the second
		// register call
		manager.UpdateStatus(unitA, unit.StatusStarted)

		// When: the unit is registered again
		err = manager.Register(unitA)

		// Then: a descriptive error should be returned
		require.ErrorContains(t, err, unit.ErrUnitAlreadyRegistered)

		// Then: the unit status should not be overwritten
		u := manager.Unit(unitA)
		assert.Equal(t, unit.StatusStarted, u.Status())
		assert.True(t, manager.IsReady(unitA))
	})

	t.Run("RegisterMultipleUnits", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Given: multiple units are registered
		unitIDs := []unit.ID{unitA, unitB, unitC}
		for _, unit := range unitIDs {
			err := manager.Register(unit)
			require.NoError(t, err)
		}

		// Then: all units should be ready initially
		for _, unitID := range unitIDs {
			u := manager.Unit(unitID)
			assert.Equal(t, unit.StatusPending, u.Status())
			assert.True(t, manager.IsReady(unitID))
		}
	})
}

func TestManager_AddDependency(t *testing.T) {
	t.Parallel()

	t.Run("AddDependencyBetweenRegisteredUnits", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Given: units A and B are registered
		err := manager.Register(unitA)
		require.NoError(t, err)
		err = manager.Register(unitB)
		require.NoError(t, err)

		// Given: Unit A depends on Unit B being unit.StatusStarted
		err = manager.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)

		// Then: Unit A should not be ready (depends on B)
		u := manager.Unit(unitA)
		assert.Equal(t, unit.StatusPending, u.Status())
		assert.False(t, manager.IsReady(unitA))

		// Then: Unit B should still be ready (no dependencies)
		u = manager.Unit(unitB)
		assert.Equal(t, unit.StatusPending, u.Status())
		assert.True(t, manager.IsReady(unitB))

		// When: Unit B is started
		err = manager.UpdateStatus(unitB, unit.StatusStarted)
		require.NoError(t, err)

		// Then: Unit A should be ready, because its dependency is now in the desired state.
		assert.True(t, manager.IsReady(unitA))

		// When: Unit B is stopped
		err = manager.UpdateStatus(unitB, unit.StatusPending)
		require.NoError(t, err)

		// Then: Unit A should no longer be ready, because its dependency is not in the desired state.
		assert.False(t, manager.IsReady(unitA))
	})

	t.Run("AddDependencyByAnUnregisteredDependentUnit", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Given Unit B is registered
		err := manager.Register(unitB)
		require.NoError(t, err)

		// Given Unit A depends on Unit B being started
		err = manager.AddDependency(unitA, unitB, unit.StatusStarted)

		// Then: a descriptive error communicates that the dependency cannot be added
		// because the dependent unit must be registered first.
		require.ErrorContains(t, err, unit.ErrUnitNotFound)
	})

	t.Run("AddDependencyOnAnUnregisteredUnit", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Given unit A is registered
		err := manager.Register(unitA)
		require.NoError(t, err)

		// Given Unit B is not yet registered
		// And Unit A depends on Unit B being started
		err = manager.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)

		u := manager.Unit(unitB)
		assert.Equal(t, unit.StatusNotRegistered, u.Status())

		// Then: Unit A should not be ready, because it depends on Unit B
		assert.False(t, manager.IsReady(unitA))

		// When: Unit B is registered
		err = manager.Register(unitB)
		require.NoError(t, err)

		// Then: Unit A should still not be ready.
		// Unit B is not registered, but it has not been started as required by the dependency.
		assert.False(t, manager.IsReady(unitA))

		// When: Unit B is started
		err = manager.UpdateStatus(unitB, unit.StatusStarted)
		require.NoError(t, err)

		// Then: Unit A should be ready, because its dependency is now in the desired state.
		assert.True(t, manager.IsReady(unitA))
	})

	t.Run("AddDependencyCreatesACyclicDependency", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Register units
		err := manager.Register(unitA)
		require.NoError(t, err)
		err = manager.Register(unitB)
		require.NoError(t, err)
		err = manager.Register(unitC)
		require.NoError(t, err)
		err = manager.Register(unitD)
		require.NoError(t, err)

		// A depends on B
		err = manager.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)
		// B depends on C
		err = manager.AddDependency(unitB, unitC, unit.StatusStarted)
		require.NoError(t, err)

		// C depends on D
		err = manager.AddDependency(unitC, unitD, unit.StatusStarted)
		require.NoError(t, err)

		// Try to make D depend on A (creates indirect cycle)
		err = manager.AddDependency(unitD, unitA, unit.StatusStarted)
		require.ErrorContains(t, err, unit.ErrCycleDetected)
	})
}

func TestManager_UpdateStatus(t *testing.T) {
	t.Parallel()

	t.Run("UpdateStatusTriggersReadinessRecalculation", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Given units A and B are registered
		err := manager.Register(unitA)
		require.NoError(t, err)
		err = manager.Register(unitB)
		require.NoError(t, err)

		// Given Unit A depends on Unit B being unit.StatusStarted
		err = manager.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)

		// Then: Unit A should not be ready (depends on B)
		u := manager.Unit(unitA)
		assert.Equal(t, unit.StatusPending, u.Status())
		assert.False(t, manager.IsReady(unitA))

		// When: Unit B is started
		err = manager.UpdateStatus(unitB, unit.StatusStarted)
		require.NoError(t, err)

		// Then: Unit A should be ready, because its dependency is now in the desired state.
		u = manager.Unit(unitA)
		assert.Equal(t, unit.StatusPending, u.Status())
		assert.True(t, manager.IsReady(unitA))
	})

	t.Run("UpdateStatusWithUnregisteredUnit", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Given Unit A is not registered
		// When: Unit A is updated to unit.StatusStarted
		err := manager.UpdateStatus(unitA, unit.StatusStarted)

		// Then: a descriptive error communicates that the unit must be registered first.
		require.ErrorContains(t, err, unit.ErrUnitNotFound)
	})

	t.Run("LinearChainDependencies", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Given units A, B, and C are registered
		err := manager.Register(unitA)
		require.NoError(t, err)
		err = manager.Register(unitB)
		require.NoError(t, err)
		err = manager.Register(unitC)
		require.NoError(t, err)

		// Create chain: A depends on B being "started", B depends on C being "completed"
		err = manager.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)
		err = manager.AddDependency(unitB, unitC, unit.StatusComplete)
		require.NoError(t, err)

		// Then: only Unit C should be ready (no dependencies)
		u := manager.Unit(unitC)
		assert.Equal(t, unit.StatusPending, u.Status())
		assert.True(t, manager.IsReady(unitC))

		u = manager.Unit(unitB)
		assert.Equal(t, unit.StatusPending, u.Status())
		assert.False(t, manager.IsReady(unitB))

		u = manager.Unit(unitA)
		assert.Equal(t, unit.StatusPending, u.Status())
		assert.False(t, manager.IsReady(unitA))

		// When: Unit C is completed
		err = manager.UpdateStatus(unitC, unit.StatusComplete)
		require.NoError(t, err)

		// Then: Unit B should be ready, because its dependency is now in the desired state.
		u = manager.Unit(unitB)
		assert.Equal(t, unit.StatusPending, u.Status())
		assert.True(t, manager.IsReady(unitB))

		u = manager.Unit(unitA)
		assert.Equal(t, unit.StatusPending, u.Status())
		assert.False(t, manager.IsReady(unitA))

		u = manager.Unit(unitB)
		assert.Equal(t, unit.StatusPending, u.Status())
		assert.True(t, manager.IsReady(unitB))

		// When: Unit B is started
		err = manager.UpdateStatus(unitB, unit.StatusStarted)
		require.NoError(t, err)

		// Then: Unit A should be ready, because its dependency is now in the desired state.
		u = manager.Unit(unitA)
		assert.Equal(t, unit.StatusPending, u.Status())
		assert.True(t, manager.IsReady(unitA))
	})
}

func TestManager_GetUnmetDependencies(t *testing.T) {
	t.Parallel()

	t.Run("GetUnmetDependenciesForUnitWithNoDependencies", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Given: Unit A is registered
		err := manager.Register(unitA)
		require.NoError(t, err)

		// Given: Unit A has no dependencies
		// Then: Unit A should have no unmet dependencies
		unmet, err := manager.GetUnmetDependencies(unitA)
		require.NoError(t, err)
		assert.Empty(t, unmet)
	})

	t.Run("GetUnmetDependenciesForUnitWithUnsatisfiedDependencies", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()
		err := manager.Register(unitA)
		require.NoError(t, err)
		err = manager.Register(unitB)
		require.NoError(t, err)

		// Given: Unit A depends on Unit B being unit.StatusStarted
		err = manager.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)

		unmet, err := manager.GetUnmetDependencies(unitA)
		require.NoError(t, err)
		require.Len(t, unmet, 1)

		assert.Equal(t, unitA, unmet[0].Unit)
		assert.Equal(t, unitB, unmet[0].DependsOn)
		assert.Equal(t, unit.StatusStarted, unmet[0].RequiredStatus)
		assert.False(t, unmet[0].IsSatisfied)
	})

	t.Run("GetUnmetDependenciesForUnitWithSatisfiedDependencies", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Given: Unit A and Unit B are registered
		err := manager.Register(unitA)
		require.NoError(t, err)
		err = manager.Register(unitB)
		require.NoError(t, err)

		// Given: Unit A depends on Unit B being unit.StatusStarted
		err = manager.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)

		// When: Unit B is started
		err = manager.UpdateStatus(unitB, unit.StatusStarted)
		require.NoError(t, err)

		// Then: Unit A should have no unmet dependencies
		unmet, err := manager.GetUnmetDependencies(unitA)
		require.NoError(t, err)
		assert.Empty(t, unmet)
	})

	t.Run("GetUnmetDependenciesForUnregisteredUnit", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// When: Unit A is requested
		unmet, err := manager.GetUnmetDependencies(unitA)

		// Then: a descriptive error communicates that the unit must be registered first.
		require.ErrorContains(t, err, unit.ErrUnitNotFound)
		assert.Nil(t, unmet)
	})
}

func TestManager_MultipleDependencies(t *testing.T) {
	t.Parallel()

	t.Run("UnitWithMultipleDependencies", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Register all units
		units := []unit.ID{unitA, unitB, unitC, unitD}
		for _, unit := range units {
			err := manager.Register(unit)
			require.NoError(t, err)
		}

		// A depends on B being unit.StatusStarted AND C being "started"
		err := manager.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)
		err = manager.AddDependency(unitA, unitC, unit.StatusStarted)
		require.NoError(t, err)

		// A should not be ready (depends on both B and C)
		assert.False(t, manager.IsReady(unitA))

		// Update B to unit.StatusStarted - A should still not be ready (needs C too)
		err = manager.UpdateStatus(unitB, unit.StatusStarted)
		require.NoError(t, err)

		assert.False(t, manager.IsReady(unitA))

		// Update C to "started" - A should now be ready
		err = manager.UpdateStatus(unitC, unit.StatusStarted)
		require.NoError(t, err)

		assert.True(t, manager.IsReady(unitA))
	})

	t.Run("ComplexDependencyChain", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Register all units
		units := []unit.ID{unitA, unitB, unitC, unitD}
		for _, unit := range units {
			err := manager.Register(unit)
			require.NoError(t, err)
		}

		// Create complex dependency graph:
		// A depends on B being unit.StatusStarted AND C being "started"
		err := manager.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)
		err = manager.AddDependency(unitA, unitC, unit.StatusStarted)
		require.NoError(t, err)
		// B depends on D being "completed"
		err = manager.AddDependency(unitB, unitD, unit.StatusComplete)
		require.NoError(t, err)
		// C depends on D being "completed"
		err = manager.AddDependency(unitC, unitD, unit.StatusComplete)
		require.NoError(t, err)

		// Initially only D is ready
		assert.True(t, manager.IsReady(unitD))
		assert.False(t, manager.IsReady(unitB))
		assert.False(t, manager.IsReady(unitC))
		assert.False(t, manager.IsReady(unitA))

		// Update D to "completed" - B and C should become ready
		err = manager.UpdateStatus(unitD, unit.StatusComplete)
		require.NoError(t, err)

		assert.True(t, manager.IsReady(unitB))
		assert.True(t, manager.IsReady(unitC))
		assert.False(t, manager.IsReady(unitA))

		// Update B to unit.StatusStarted - A should still not be ready (needs C)
		err = manager.UpdateStatus(unitB, unit.StatusStarted)
		require.NoError(t, err)

		assert.False(t, manager.IsReady(unitA))

		// Update C to "started" - A should now be ready
		err = manager.UpdateStatus(unitC, unit.StatusStarted)
		require.NoError(t, err)

		assert.True(t, manager.IsReady(unitA))
	})

	t.Run("DifferentStatusTypes", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Register units
		err := manager.Register(unitA)
		require.NoError(t, err)
		err = manager.Register(unitB)
		require.NoError(t, err)
		err = manager.Register(unitC)
		require.NoError(t, err)

		// Given: Unit A depends on Unit B being unit.StatusStarted
		err = manager.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)
		// Given: Unit A depends on Unit C being "completed"
		err = manager.AddDependency(unitA, unitC, unit.StatusComplete)
		require.NoError(t, err)

		// When: Unit B is started
		err = manager.UpdateStatus(unitB, unit.StatusStarted)
		require.NoError(t, err)

		// Then: Unit A should not be ready, because only one of its dependencies is in the desired state.
		// It still requires Unit C to be completed.
		assert.False(t, manager.IsReady(unitA))

		// When: Unit C is completed
		err = manager.UpdateStatus(unitC, unit.StatusComplete)
		require.NoError(t, err)

		// Then: Unit A should be ready, because both of its dependencies are in the desired state.
		assert.True(t, manager.IsReady(unitA))
	})
}

func TestManager_IsReady(t *testing.T) {
	t.Parallel()

	t.Run("IsReadyWithUnregisteredUnit", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Given: a unit is not registered
		u := manager.Unit(unitA)
		assert.Equal(t, unit.StatusNotRegistered, u.Status())
		// Then: the unit is not ready
		assert.False(t, manager.IsReady(unitA))
	})
}

func TestManager_ToDOT(t *testing.T) {
	t.Parallel()

	t.Run("ExportSimpleGraph", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Register units
		err := manager.Register(unitA)
		require.NoError(t, err)
		err = manager.Register(unitB)
		require.NoError(t, err)

		// Add dependency
		err = manager.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)

		dot, err := manager.ExportDOT("test")
		require.NoError(t, err)
		assert.NotEmpty(t, dot)
		assert.Contains(t, dot, "digraph")
	})

	t.Run("ExportComplexGraph", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()

		// Register all units
		units := []unit.ID{unitA, unitB, unitC, unitD}
		for _, unit := range units {
			err := manager.Register(unit)
			require.NoError(t, err)
		}

		// Create complex dependency graph
		// A depends on B and C, B depends on D, C depends on D
		err := manager.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)
		err = manager.AddDependency(unitA, unitC, unit.StatusStarted)
		require.NoError(t, err)
		err = manager.AddDependency(unitB, unitD, unit.StatusComplete)
		require.NoError(t, err)
		err = manager.AddDependency(unitC, unitD, unit.StatusComplete)
		require.NoError(t, err)

		dot, err := manager.ExportDOT("complex")
		require.NoError(t, err)
		assert.NotEmpty(t, dot)
		assert.Contains(t, dot, "digraph")
	})
}
