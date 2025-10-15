package agent_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/testutil"
)

func TestUnitCoordinator(t *testing.T) {
	t.Parallel()

	t.Run("StartUnit", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryUnitCoordinator()

		// Start unit
		acquired := coordinator.StartUnit("test.unit")
		assert.True(t, acquired)

		// Cannot start a second unit with the same name while the first is running
		acquired = coordinator.StartUnit("test.unit")
		assert.False(t, acquired)

		// Check unit is running
		assert.True(t, coordinator.IsUnitHeld("test.unit"))
	})

	t.Run("StopUnit", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryUnitCoordinator()

		// Start unit
		acquired := coordinator.StartUnit("test.unit")
		assert.True(t, acquired)
		assert.True(t, coordinator.IsUnitHeld("test.unit"))

		// Stop unit
		coordinator.StopUnit("test.unit")
		assert.False(t, coordinator.IsUnitHeld("test.unit"))

		// Can start unit again after stopping
		acquired = coordinator.StartUnit("test.unit")
		assert.True(t, acquired)
		assert.True(t, coordinator.IsUnitHeld("test.unit"))
	})

	t.Run("SubscribeToLock", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryUnitCoordinator()

		received := make(chan agent.UnitEventType, 10)
		cancel, err := coordinator.SubscribeToUnit("test.unit", func(ctx context.Context, event agent.UnitEventType) {
			received <- event
		})
		require.NoError(t, err)
		defer cancel()

		ctx := testutil.Context(t, testutil.WaitShort)

		// Acquire lock - should trigger acquired event
		coordinator.StartUnit("test.unit")
		event := testutil.RequireReceive(ctx, t, received)
		assert.Equal(t, agent.UnitEventTypeAcquired, event)

		// Release lock - should trigger released event
		coordinator.StopUnit("test.unit")
		event = testutil.RequireReceive(ctx, t, received)
		assert.Equal(t, agent.UnitEventTypeReleased, event)
	})

	t.Run("SubscribeToLockAfterAcquire", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryUnitCoordinator()

		// Acquire lock first
		coordinator.StartUnit("test.unit")

		// Subscribe after acquisition - should receive historical event
		received := make(chan agent.UnitEventType, 10)
		cancel, err := coordinator.SubscribeToUnit("test.unit", func(ctx context.Context, event agent.UnitEventType) {
			received <- event
		})
		require.NoError(t, err)
		defer cancel()

		ctx := testutil.Context(t, testutil.WaitShort)
		event := testutil.RequireReceive(ctx, t, received)
		assert.Equal(t, agent.UnitEventTypeAcquired, event)
	})

	t.Run("GetLockHistory", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryUnitCoordinator()

		// Initially no history
		history := coordinator.GetUnitHistory("test.unit")
		assert.Empty(t, history)

		// Acquire lock
		coordinator.StartUnit("test.unit")
		history = coordinator.GetUnitHistory("test.unit")
		require.Len(t, history, 1)
		assert.Equal(t, agent.UnitEventTypeAcquired, history[0].Type)

		// Release lock
		coordinator.StopUnit("test.unit")
		history = coordinator.GetUnitHistory("test.unit")
		require.Len(t, history, 2)
		assert.Equal(t, agent.UnitEventTypeAcquired, history[0].Type)
		assert.Equal(t, agent.UnitEventTypeReleased, history[1].Type)

		// Acquire again
		coordinator.StartUnit("test.unit")
		history = coordinator.GetUnitHistory("test.unit")
		require.Len(t, history, 3)
		assert.Equal(t, agent.UnitEventTypeAcquired, history[2].Type)
	})

	t.Run("MultipleLocks", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryUnitCoordinator()

		// Acquire different locks
		assert.True(t, coordinator.StartUnit("unit1"))
		assert.True(t, coordinator.StartUnit("unit2"))
		assert.True(t, coordinator.StartUnit("unit3"))

		// All should be held
		assert.True(t, coordinator.IsUnitHeld("unit1"))
		assert.True(t, coordinator.IsUnitHeld("unit2"))
		assert.True(t, coordinator.IsUnitHeld("unit3"))

		// Release one
		coordinator.StopUnit("unit2")
		assert.True(t, coordinator.IsUnitHeld("unit1"))
		assert.False(t, coordinator.IsUnitHeld("unit2"))
		assert.True(t, coordinator.IsUnitHeld("unit3"))
	})

	t.Run("ConcurrentAcquisition", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryUnitCoordinator()

		// Simulate concurrent acquisition attempts
		results := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				results <- coordinator.StartUnit("concurrent.unit")
			}()
		}

		// Only one should succeed
		successCount := 0
		for i := 0; i < 10; i++ {
			if <-results {
				successCount++
			}
		}

		assert.Equal(t, 1, successCount)
		assert.True(t, coordinator.IsUnitHeld("concurrent.unit"))
	})

	t.Run("Close", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryUnitCoordinator()

		// Acquire a lock
		coordinator.StartUnit("test.unit")
		assert.True(t, coordinator.IsUnitHeld("test.unit"))

		// Close coordinator
		err := coordinator.Close()
		assert.NoError(t, err)

		// Should not be able to acquire locks after close
		acquired := coordinator.StartUnit("test.unit")
		assert.False(t, acquired)

		// Should not be able to subscribe after close
		_, err = coordinator.SubscribeToUnit("test.unit", func(ctx context.Context, event agent.UnitEventType) {})
		assert.Error(t, err)
	})
}
