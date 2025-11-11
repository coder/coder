package unit_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/unit"
)

type testStatus string

const (
	statusStarted   testStatus = "started"
	statusRunning   testStatus = "running"
	statusCompleted testStatus = "completed"
)

type testConsumerID string

const (
	consumerA testConsumerID = "serviceA"
	consumerB testConsumerID = "serviceB"
	consumerC testConsumerID = "serviceC"
	consumerD testConsumerID = "serviceD"
)

func TestDependencyTracker_Register(t *testing.T) {
	t.Parallel()

	tracker := unit.NewManager[testStatus, testConsumerID]()

	t.Run("RegisterNewConsumer", func(t *testing.T) {
		t.Parallel()

		err := tracker.Register(consumerA)
		require.NoError(t, err)

		// Consumer should be ready initially (no dependencies)
		ready, err := tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.True(t, ready)
	})

	t.Run("RegisterDuplicateConsumer", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()
		err := tracker.Register(consumerA)
		require.NoError(t, err)

		err = tracker.Register(consumerA)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("RegisterMultipleConsumers", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		consumers := []testConsumerID{consumerA, consumerB, consumerC}
		for _, consumer := range consumers {
			err := tracker.Register(consumer)
			require.NoError(t, err)
		}

		// All should be ready initially
		for _, consumer := range consumers {
			ready, err := tracker.IsReady(consumer)
			require.NoError(t, err)
			assert.True(t, ready)
		}
	})
}

func TestDependencyTracker_AddDependency(t *testing.T) {
	t.Parallel()

	t.Run("AddDependencyBetweenRegisteredConsumers", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()
		err := tracker.Register(consumerA)
		require.NoError(t, err)
		err = tracker.Register(consumerB)
		require.NoError(t, err)

		// A depends on B being "running"
		err = tracker.AddDependency(consumerA, consumerB, statusRunning)
		require.NoError(t, err)

		// A should no longer be ready (depends on B)
		ready, err := tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.False(t, ready)

		// B should still be ready (no dependencies)
		ready, err = tracker.IsReady(consumerB)
		require.NoError(t, err)
		assert.True(t, ready)
	})

	t.Run("AddDependencyWithUnregisteredConsumer", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()
		err := tracker.Register(consumerA)
		require.NoError(t, err)

		// Try to add dependency to unregistered consumer
		err = tracker.AddDependency(consumerA, consumerB, statusRunning)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not registered")
	})

	t.Run("AddDependencyFromUnregisteredConsumer", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()
		err := tracker.Register(consumerB)
		require.NoError(t, err)

		// Try to add dependency from unregistered consumer
		err = tracker.AddDependency(consumerA, consumerB, statusRunning)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not registered")
	})
}

func TestDependencyTracker_UpdateStatus(t *testing.T) {
	t.Parallel()

	t.Run("UpdateStatusTriggersReadinessRecalculation", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()
		err := tracker.Register(consumerA)
		require.NoError(t, err)
		err = tracker.Register(consumerB)
		require.NoError(t, err)

		// A depends on B being "running"
		err = tracker.AddDependency(consumerA, consumerB, statusRunning)
		require.NoError(t, err)

		// Initially A is not ready
		ready, err := tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update B to "running" - A should become ready
		err = tracker.UpdateStatus(consumerB, statusRunning)
		require.NoError(t, err)

		ready, err = tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.True(t, ready)
	})

	t.Run("UpdateStatusWithUnregisteredConsumer", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		err := tracker.UpdateStatus(consumerA, statusRunning)
		require.Error(t, err)
		assert.Equal(t, unit.ErrConsumerNotFound, err)
	})

	t.Run("LinearChainDependencies", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		// Register all consumers
		consumers := []testConsumerID{consumerA, consumerB, consumerC}
		for _, consumer := range consumers {
			err := tracker.Register(consumer)
			require.NoError(t, err)
		}

		// Create chain: A depends on B being "started", B depends on C being "completed"
		err := tracker.AddDependency(consumerA, consumerB, statusStarted)
		require.NoError(t, err)
		err = tracker.AddDependency(consumerB, consumerC, statusCompleted)
		require.NoError(t, err)

		// Initially only C is ready
		ready, err := tracker.IsReady(consumerC)
		require.NoError(t, err)
		assert.True(t, ready)

		ready, err = tracker.IsReady(consumerB)
		require.NoError(t, err)
		assert.False(t, ready)

		ready, err = tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update C to "completed" - B should become ready
		err = tracker.UpdateStatus(consumerC, statusCompleted)
		require.NoError(t, err)

		ready, err = tracker.IsReady(consumerB)
		require.NoError(t, err)
		assert.True(t, ready)

		ready, err = tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update B to "started" - A should become ready
		err = tracker.UpdateStatus(consumerB, statusStarted)
		require.NoError(t, err)

		ready, err = tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.True(t, ready)
	})
}

func TestDependencyTracker_GetUnmetDependencies(t *testing.T) {
	t.Parallel()

	t.Run("GetUnmetDependenciesForConsumerWithNoDependencies", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()
		err := tracker.Register(consumerA)
		require.NoError(t, err)

		unmet, err := tracker.GetUnmetDependencies(consumerA)
		require.NoError(t, err)
		assert.Empty(t, unmet)
	})

	t.Run("GetUnmetDependenciesForConsumerWithUnsatisfiedDependencies", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()
		err := tracker.Register(consumerA)
		require.NoError(t, err)
		err = tracker.Register(consumerB)
		require.NoError(t, err)

		// A depends on B being "running"
		err = tracker.AddDependency(consumerA, consumerB, statusRunning)
		require.NoError(t, err)

		unmet, err := tracker.GetUnmetDependencies(consumerA)
		require.NoError(t, err)
		require.Len(t, unmet, 1)

		assert.Equal(t, consumerA, unmet[0].Consumer)
		assert.Equal(t, consumerB, unmet[0].DependsOn)
		assert.Equal(t, statusRunning, unmet[0].RequiredStatus)
		assert.False(t, unmet[0].IsSatisfied)
	})

	t.Run("GetUnmetDependenciesForConsumerWithSatisfiedDependencies", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()
		err := tracker.Register(consumerA)
		require.NoError(t, err)
		err = tracker.Register(consumerB)
		require.NoError(t, err)

		// A depends on B being "running"
		err = tracker.AddDependency(consumerA, consumerB, statusRunning)
		require.NoError(t, err)

		// Update B to "running"
		err = tracker.UpdateStatus(consumerB, statusRunning)
		require.NoError(t, err)

		unmet, err := tracker.GetUnmetDependencies(consumerA)
		require.NoError(t, err)
		assert.Empty(t, unmet)
	})

	t.Run("GetUnmetDependenciesForUnregisteredConsumer", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		unmet, err := tracker.GetUnmetDependencies(consumerA)
		require.Error(t, err)
		assert.Equal(t, unit.ErrConsumerNotFound, err)
		assert.Nil(t, unmet)
	})
}

func TestDependencyTracker_ConcurrentOperations(t *testing.T) {
	t.Parallel()

	t.Run("ConcurrentStatusUpdates", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		// Register consumers
		consumers := []testConsumerID{consumerA, consumerB, consumerC, consumerD}
		for _, consumer := range consumers {
			err := tracker.Register(consumer)
			require.NoError(t, err)
		}

		// Create dependencies: A depends on B, B depends on C, C depends on D
		err := tracker.AddDependency(consumerA, consumerB, statusRunning)
		require.NoError(t, err)
		err = tracker.AddDependency(consumerB, consumerC, statusStarted)
		require.NoError(t, err)
		err = tracker.AddDependency(consumerC, consumerD, statusCompleted)
		require.NoError(t, err)

		var wg sync.WaitGroup
		const numGoroutines = 10

		// Launch goroutines that update statuses
		goroutineErrors := make([]error, numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()

				// Update D to completed (should make C ready)
				err := tracker.UpdateStatus(consumerD, statusCompleted)
				if err != nil {
					goroutineErrors[goroutineID] = err
					return
				}

				// Update C to started (should make B ready)
				err = tracker.UpdateStatus(consumerC, statusStarted)
				if err != nil {
					goroutineErrors[goroutineID] = err
					return
				}

				// Update B to running (should make A ready)
				err = tracker.UpdateStatus(consumerB, statusRunning)
				if err != nil {
					goroutineErrors[goroutineID] = err
					return
				}
			}(i)
		}

		wg.Wait()

		// Check for any errors in goroutines
		// ErrSameStatusAlreadySet is expected when multiple goroutines try to set the same status
		for i, err := range goroutineErrors {
			if err != nil && !errors.Is(err, unit.ErrSameStatusAlreadySet) {
				require.NoError(t, err, "goroutine %d had unexpected error", i)
			}
		}

		// All consumers should be ready after the updates
		for _, consumer := range consumers {
			ready, err := tracker.IsReady(consumer)
			require.NoError(t, err)
			assert.True(t, ready)
		}
	})

	t.Run("ConcurrentReadinessChecks", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		// Register consumers
		err := tracker.Register(consumerA)
		require.NoError(t, err)
		err = tracker.Register(consumerB)
		require.NoError(t, err)

		// A depends on B being "running"
		err = tracker.AddDependency(consumerA, consumerB, statusRunning)
		require.NoError(t, err)

		var wg sync.WaitGroup
		const numGoroutines = 20

		// Launch goroutines that check readiness
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()

				// Check readiness multiple times
				for j := 0; j < 10; j++ {
					ready, err := tracker.IsReady(consumerA)
					require.NoError(t, err)
					// Initially should be false, then true after B is updated
					_ = ready

					ready, err = tracker.IsReady(consumerB)
					require.NoError(t, err)
					// B should always be ready (no dependencies)
					assert.True(t, ready)
				}
			}(i)
		}

		// Update B to "running" in the middle of readiness checks
		err = tracker.UpdateStatus(consumerB, statusRunning)
		require.NoError(t, err)

		wg.Wait()
	})
}

func TestDependencyTracker_MultipleDependencies(t *testing.T) {
	t.Parallel()

	t.Run("ConsumerWithMultipleDependencies", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		// Register all consumers
		consumers := []testConsumerID{consumerA, consumerB, consumerC, consumerD}
		for _, consumer := range consumers {
			err := tracker.Register(consumer)
			require.NoError(t, err)
		}

		// A depends on B being "running" AND C being "started"
		err := tracker.AddDependency(consumerA, consumerB, statusRunning)
		require.NoError(t, err)
		err = tracker.AddDependency(consumerA, consumerC, statusStarted)
		require.NoError(t, err)

		// A should not be ready (depends on both B and C)
		ready, err := tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update B to "running" - A should still not be ready (needs C too)
		err = tracker.UpdateStatus(consumerB, statusRunning)
		require.NoError(t, err)

		ready, err = tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update C to "started" - A should now be ready
		err = tracker.UpdateStatus(consumerC, statusStarted)
		require.NoError(t, err)

		ready, err = tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.True(t, ready)
	})

	t.Run("ComplexDependencyChain", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		// Register all consumers
		consumers := []testConsumerID{consumerA, consumerB, consumerC, consumerD}
		for _, consumer := range consumers {
			err := tracker.Register(consumer)
			require.NoError(t, err)
		}

		// Create complex dependency graph:
		// A depends on B being "running" AND C being "started"
		// B depends on D being "completed"
		// C depends on D being "completed"
		err := tracker.AddDependency(consumerA, consumerB, statusRunning)
		require.NoError(t, err)
		err = tracker.AddDependency(consumerA, consumerC, statusStarted)
		require.NoError(t, err)
		err = tracker.AddDependency(consumerB, consumerD, statusCompleted)
		require.NoError(t, err)
		err = tracker.AddDependency(consumerC, consumerD, statusCompleted)
		require.NoError(t, err)

		// Initially only D is ready
		ready, err := tracker.IsReady(consumerD)
		require.NoError(t, err)
		assert.True(t, ready)

		ready, err = tracker.IsReady(consumerB)
		require.NoError(t, err)
		assert.False(t, ready)

		ready, err = tracker.IsReady(consumerC)
		require.NoError(t, err)
		assert.False(t, ready)

		ready, err = tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update D to "completed" - B and C should become ready
		err = tracker.UpdateStatus(consumerD, statusCompleted)
		require.NoError(t, err)

		ready, err = tracker.IsReady(consumerB)
		require.NoError(t, err)
		assert.True(t, ready)

		ready, err = tracker.IsReady(consumerC)
		require.NoError(t, err)
		assert.True(t, ready)

		ready, err = tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update B to "running" - A should still not be ready (needs C)
		err = tracker.UpdateStatus(consumerB, statusRunning)
		require.NoError(t, err)

		ready, err = tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update C to "started" - A should now be ready
		err = tracker.UpdateStatus(consumerC, statusStarted)
		require.NoError(t, err)

		ready, err = tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.True(t, ready)
	})

	t.Run("DifferentStatusTypes", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		// Register consumers
		err := tracker.Register(consumerA)
		require.NoError(t, err)
		err = tracker.Register(consumerB)
		require.NoError(t, err)
		err = tracker.Register(consumerC)
		require.NoError(t, err)

		// A depends on B being "running" AND C being "completed"
		err = tracker.AddDependency(consumerA, consumerB, statusRunning)
		require.NoError(t, err)
		err = tracker.AddDependency(consumerA, consumerC, statusCompleted)
		require.NoError(t, err)

		// Update B to "running" but not C - A should not be ready
		err = tracker.UpdateStatus(consumerB, statusRunning)
		require.NoError(t, err)

		ready, err := tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.False(t, ready)

		// Update C to "completed" - A should now be ready
		err = tracker.UpdateStatus(consumerC, statusCompleted)
		require.NoError(t, err)

		ready, err = tracker.IsReady(consumerA)
		require.NoError(t, err)
		assert.True(t, ready)
	})
}

func TestDependencyTracker_ErrorCases(t *testing.T) {
	t.Parallel()

	t.Run("UpdateStatusWithUnregisteredConsumer", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		err := tracker.UpdateStatus(consumerA, statusRunning)
		require.Error(t, err)
		assert.Equal(t, unit.ErrConsumerNotFound, err)
	})

	t.Run("IsReadyWithUnregisteredConsumer", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		ready, err := tracker.IsReady(consumerA)
		require.Error(t, err)
		assert.Equal(t, unit.ErrConsumerNotFound, err)
		assert.False(t, ready)
	})

	t.Run("GetUnmetDependenciesWithUnregisteredConsumer", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		unmet, err := tracker.GetUnmetDependencies(consumerA)
		require.Error(t, err)
		assert.Equal(t, unit.ErrConsumerNotFound, err)
		assert.Nil(t, unmet)
	})

	t.Run("AddDependencyWithUnregisteredConsumers", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		// Try to add dependency with unregistered consumers
		err := tracker.AddDependency(consumerA, consumerB, statusRunning)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not registered")
	})

	t.Run("CyclicDependencyDetection", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		// Register consumers
		err := tracker.Register(consumerA)
		require.NoError(t, err)
		err = tracker.Register(consumerB)
		require.NoError(t, err)

		// A depends on B
		err = tracker.AddDependency(consumerA, consumerB, statusRunning)
		require.NoError(t, err)

		// Try to make B depend on A (creates cycle)
		err = tracker.AddDependency(consumerB, consumerA, statusStarted)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "would create a cycle")
	})
}

func TestDependencyTracker_ToDOT(t *testing.T) {
	t.Parallel()

	t.Run("ExportSimpleGraph", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		// Register consumers
		err := tracker.Register(consumerA)
		require.NoError(t, err)
		err = tracker.Register(consumerB)
		require.NoError(t, err)

		// Add dependency
		err = tracker.AddDependency(consumerA, consumerB, statusRunning)
		require.NoError(t, err)

		dot, err := tracker.ExportDOT("test")
		require.NoError(t, err)
		assert.NotEmpty(t, dot)
		assert.Contains(t, dot, "digraph")
	})

	t.Run("ExportComplexGraph", func(t *testing.T) {
		t.Parallel()

		tracker := unit.NewManager[testStatus, testConsumerID]()

		// Register all consumers
		consumers := []testConsumerID{consumerA, consumerB, consumerC, consumerD}
		for _, consumer := range consumers {
			err := tracker.Register(consumer)
			require.NoError(t, err)
		}

		// Create complex dependency graph
		// A depends on B and C, B depends on D, C depends on D
		err := tracker.AddDependency(consumerA, consumerB, statusRunning)
		require.NoError(t, err)
		err = tracker.AddDependency(consumerA, consumerC, statusStarted)
		require.NoError(t, err)
		err = tracker.AddDependency(consumerB, consumerD, statusCompleted)
		require.NoError(t, err)
		err = tracker.AddDependency(consumerC, consumerD, statusCompleted)
		require.NoError(t, err)

		dot, err := tracker.ExportDOT("complex")
		require.NoError(t, err)
		assert.NotEmpty(t, dot)
		assert.Contains(t, dot, "digraph")
	})
}
