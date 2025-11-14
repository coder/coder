package unit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/unit"
)

type testUnitID string

const (
	unitA testUnitID = "serviceA"
	unitB testUnitID = "serviceB"
	unitC testUnitID = "serviceC"
	unitD testUnitID = "serviceD"
)

func TestDependencyTracker_Register(t *testing.T) {
	t.Parallel()

	t.Run("RegisterNewUnit", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()
		err := tracker.Register(unitA)
		require.NoError(t, err)

		// Unit should be ready initially (no dependencies)
		ready, err := tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.True(t, ready)
	})

	t.Run("RegisterDuplicateUnit", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()
		err := tracker.Register(unitA)
		require.NoError(t, err)

		err = tracker.Register(unitA)
		require.ErrorIs(t, err, unit.ErrUnitAlreadyRegistered)
	})

	t.Run("RegisterMultipleUnits", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()

		units := []testUnitID{unitA, unitB, unitC}
		for _, unit := range units {
			err := tracker.Register(unit)
			require.NoError(t, err)
		}

		// All should be ready initially
		for _, unit := range units {
			ready, err := tracker.IsReady(unit)
			require.NoError(t, err)
			assert.True(t, ready)
		}
	})
}

func TestDependencyTracker_AddDependency(t *testing.T) {
	t.Parallel()

	t.Run("AddDependencyBetweenRegisteredUnits", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()
		err := tracker.Register(unitA)
		require.NoError(t, err)
		err = tracker.Register(unitB)
		require.NoError(t, err)

		// A depends on B being unit.StatusStarted
		err = tracker.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)

		// A should no longer be ready (depends on B)
		ready, err := tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.False(t, ready)

		// B should still be ready (no dependencies)
		ready, err = tracker.IsReady(unitB)
		require.NoError(t, err)
		assert.True(t, ready)
	})

	t.Run("AddDependencyWithUnregisteredUnit", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()
		err := tracker.Register(unitA)
		require.NoError(t, err)

		// Try to add dependency to unregistered unit
		err = tracker.AddDependency(unitA, unitB, unit.StatusStarted)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)
	})

	t.Run("AddDependencyFromUnregisteredUnit", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()
		err := tracker.Register(unitB)
		require.NoError(t, err)

		// Try to add dependency from unregistered unit
		err = tracker.AddDependency(unitA, unitB, unit.StatusStarted)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)
	})
}

func TestDependencyTracker_UpdateStatus(t *testing.T) {
	t.Parallel()

	t.Run("UpdateStatusTriggersReadinessRecalculation", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()
		err := tracker.Register(unitA)
		require.NoError(t, err)
		err = tracker.Register(unitB)
		require.NoError(t, err)

		// A depends on B being unit.StatusStarted
		err = tracker.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)

		// Initially A is not ready
		ready, err := tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update B to unit.StatusStarted - A should become ready
		err = tracker.UpdateStatus(unitB, unit.StatusStarted)
		require.NoError(t, err)

		ready, err = tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.True(t, ready)
	})

	t.Run("UpdateStatusWithUnregisteredUnit", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()

		err := tracker.UpdateStatus(unitA, unit.StatusStarted)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)
	})

	t.Run("LinearChainDependencies", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()

		// Register all units
		units := []testUnitID{unitA, unitB, unitC}
		for _, unit := range units {
			err := tracker.Register(unit)
			require.NoError(t, err)
		}

		// Create chain: A depends on B being "started", B depends on C being "completed"
		err := tracker.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)
		err = tracker.AddDependency(unitB, unitC, unit.StatusComplete)
		require.NoError(t, err)

		// Initially only C is ready
		ready, err := tracker.IsReady(unitC)
		require.NoError(t, err)
		assert.True(t, ready)

		ready, err = tracker.IsReady(unitB)
		require.NoError(t, err)
		assert.False(t, ready)

		ready, err = tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update C to "completed" - B should become ready
		err = tracker.UpdateStatus(unitC, unit.StatusComplete)
		require.NoError(t, err)

		ready, err = tracker.IsReady(unitB)
		require.NoError(t, err)
		assert.True(t, ready)

		ready, err = tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update B to "started" - A should become ready
		err = tracker.UpdateStatus(unitB, unit.StatusStarted)
		require.NoError(t, err)

		ready, err = tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.True(t, ready)
	})
}

func TestDependencyTracker_GetUnmetDependencies(t *testing.T) {
	t.Parallel()

	t.Run("GetUnmetDependenciesForUnitWithNoDependencies", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()
		err := tracker.Register(unitA)
		require.NoError(t, err)

		unmet, err := tracker.GetUnmetDependencies(unitA)
		require.NoError(t, err)
		assert.Empty(t, unmet)
	})

	t.Run("GetUnmetDependenciesForUnitWithUnsatisfiedDependencies", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()
		err := tracker.Register(unitA)
		require.NoError(t, err)
		err = tracker.Register(unitB)
		require.NoError(t, err)

		// A depends on B being unit.StatusStarted
		err = tracker.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)

		unmet, err := tracker.GetUnmetDependencies(unitA)
		require.NoError(t, err)
		require.Len(t, unmet, 1)

		assert.Equal(t, unitA, unmet[0].Unit)
		assert.Equal(t, unitB, unmet[0].DependsOn)
		assert.Equal(t, unit.StatusStarted, unmet[0].RequiredStatus)
		assert.False(t, unmet[0].IsSatisfied)
	})

	t.Run("GetUnmetDependenciesForUnitWithSatisfiedDependencies", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()
		err := tracker.Register(unitA)
		require.NoError(t, err)
		err = tracker.Register(unitB)
		require.NoError(t, err)

		// A depends on B being unit.StatusStarted
		err = tracker.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)

		// Update B to unit.StatusStarted
		err = tracker.UpdateStatus(unitB, unit.StatusStarted)
		require.NoError(t, err)

		unmet, err := tracker.GetUnmetDependencies(unitA)
		require.NoError(t, err)
		assert.Empty(t, unmet)
	})

	t.Run("GetUnmetDependenciesForUnregisteredUnit", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()

		unmet, err := tracker.GetUnmetDependencies(unitA)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)
		assert.Nil(t, unmet)
	})
}

func TestDependencyTracker_MultipleDependencies(t *testing.T) {
	t.Parallel()

	t.Run("UnitWithMultipleDependencies", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()

		// Register all units
		units := []testUnitID{unitA, unitB, unitC, unitD}
		for _, unit := range units {
			err := tracker.Register(unit)
			require.NoError(t, err)
		}

		// A depends on B being unit.StatusStarted AND C being "started"
		err := tracker.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)
		err = tracker.AddDependency(unitA, unitC, unit.StatusStarted)
		require.NoError(t, err)

		// A should not be ready (depends on both B and C)
		ready, err := tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update B to unit.StatusStarted - A should still not be ready (needs C too)
		err = tracker.UpdateStatus(unitB, unit.StatusStarted)
		require.NoError(t, err)

		ready, err = tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update C to "started" - A should now be ready
		err = tracker.UpdateStatus(unitC, unit.StatusStarted)
		require.NoError(t, err)

		ready, err = tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.True(t, ready)
	})

	t.Run("ComplexDependencyChain", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()

		// Register all units
		units := []testUnitID{unitA, unitB, unitC, unitD}
		for _, unit := range units {
			err := tracker.Register(unit)
			require.NoError(t, err)
		}

		// Create complex dependency graph:
		// A depends on B being unit.StatusStarted AND C being "started"
		// B depends on D being "completed"
		// C depends on D being "completed"
		err := tracker.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)
		err = tracker.AddDependency(unitA, unitC, unit.StatusStarted)
		require.NoError(t, err)
		err = tracker.AddDependency(unitB, unitD, unit.StatusComplete)
		require.NoError(t, err)
		err = tracker.AddDependency(unitC, unitD, unit.StatusComplete)
		require.NoError(t, err)

		// Initially only D is ready
		ready, err := tracker.IsReady(unitD)
		require.NoError(t, err)
		assert.True(t, ready)

		ready, err = tracker.IsReady(unitB)
		require.NoError(t, err)
		assert.False(t, ready)

		ready, err = tracker.IsReady(unitC)
		require.NoError(t, err)
		assert.False(t, ready)

		ready, err = tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update D to "completed" - B and C should become ready
		err = tracker.UpdateStatus(unitD, unit.StatusComplete)
		require.NoError(t, err)

		ready, err = tracker.IsReady(unitB)
		require.NoError(t, err)
		assert.True(t, ready)

		ready, err = tracker.IsReady(unitC)
		require.NoError(t, err)
		assert.True(t, ready)

		ready, err = tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update B to unit.StatusStarted - A should still not be ready (needs C)
		err = tracker.UpdateStatus(unitB, unit.StatusStarted)
		require.NoError(t, err)

		ready, err = tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update C to "started" - A should now be ready
		err = tracker.UpdateStatus(unitC, unit.StatusStarted)
		require.NoError(t, err)

		ready, err = tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.True(t, ready)
	})

	t.Run("DifferentStatusTypes", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()

		// Register units
		err := tracker.Register(unitA)
		require.NoError(t, err)
		err = tracker.Register(unitB)
		require.NoError(t, err)
		err = tracker.Register(unitC)
		require.NoError(t, err)

		// A depends on B being unit.StatusStarted AND C being "completed"
		err = tracker.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)
		err = tracker.AddDependency(unitA, unitC, unit.StatusComplete)
		require.NoError(t, err)

		// Update B to unit.StatusStarted but not C - A should not be ready
		err = tracker.UpdateStatus(unitB, unit.StatusStarted)
		require.NoError(t, err)

		ready, err := tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update C to "completed" - A should now be ready
		err = tracker.UpdateStatus(unitC, unit.StatusComplete)
		require.NoError(t, err)

		ready, err = tracker.IsReady(unitA)
		require.NoError(t, err)
		assert.True(t, ready)
	})
}

func TestDependencyTracker_ErrorCases(t *testing.T) {
	t.Parallel()

	t.Run("UpdateStatusWithUnregisteredUnit", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()

		err := tracker.UpdateStatus(unitA, unit.StatusStarted)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)
	})

	t.Run("IsReadyWithUnregisteredUnit", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()

		ready, err := tracker.IsReady(unitA)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)
		assert.False(t, ready)
	})

	t.Run("GetUnmetDependenciesWithUnregisteredUnit", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()

		unmet, err := tracker.GetUnmetDependencies(unitA)
		require.Error(t, err)
		assert.Equal(t, unit.ErrUnitNotFound, err)
		assert.Nil(t, unmet)
	})

	t.Run("AddDependencyWithUnregisteredUnits", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()

		// Try to add dependency with unregistered units
		err := tracker.AddDependency(unitA, unitB, unit.StatusStarted)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not registered")
	})

	t.Run("CyclicDependencyDetection", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()

		// Register units
		err := tracker.Register(unitA)
		require.NoError(t, err)
		err = tracker.Register(unitB)
		require.NoError(t, err)
		err = tracker.Register(unitC)
		require.NoError(t, err)
		err = tracker.Register(unitD)
		require.NoError(t, err)

		// A depends on B
		err = tracker.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)
		// B depends on C
		err = tracker.AddDependency(unitB, unitC, unit.StatusStarted)
		require.NoError(t, err)

		// C depends on D
		err = tracker.AddDependency(unitC, unitD, unit.StatusStarted)
		require.NoError(t, err)

		// Try to make D depend on A (creates indirect cycle)
		err = tracker.AddDependency(unitD, unitA, unit.StatusStarted)
		require.ErrorIs(t, err, unit.ErrCycleDetected)
	})
}

func TestDependencyTracker_ToDOT(t *testing.T) {
	t.Parallel()

	t.Run("ExportSimpleGraph", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()

		// Register units
		err := tracker.Register(unitA)
		require.NoError(t, err)
		err = tracker.Register(unitB)
		require.NoError(t, err)

		// Add dependency
		err = tracker.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)

		dot, err := tracker.ExportDOT("test")
		require.NoError(t, err)
		assert.NotEmpty(t, dot)
		assert.Contains(t, dot, "digraph")
	})

	t.Run("ExportComplexGraph", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testUnitID]()

		// Register all units
		units := []testUnitID{unitA, unitB, unitC, unitD}
		for _, unit := range units {
			err := tracker.Register(unit)
			require.NoError(t, err)
		}

		// Create complex dependency graph
		// A depends on B and C, B depends on D, C depends on D
		err := tracker.AddDependency(unitA, unitB, unit.StatusStarted)
		require.NoError(t, err)
		err = tracker.AddDependency(unitA, unitC, unit.StatusStarted)
		require.NoError(t, err)
		err = tracker.AddDependency(unitB, unitD, unit.StatusComplete)
		require.NoError(t, err)
		err = tracker.AddDependency(unitC, unitD, unit.StatusComplete)
		require.NoError(t, err)

		dot, err := tracker.ExportDOT("complex")
		require.NoError(t, err)
		assert.NotEmpty(t, dot)
		assert.Contains(t, dot, "digraph")
	})
}
