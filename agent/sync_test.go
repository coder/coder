package agent_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/testutil"
)

func TestLockCoordinator(t *testing.T) {
	t.Parallel()

	t.Run("AcquireLock", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryLockCoordinator()

		// First acquisition should succeed
		acquired := coordinator.AcquireLock("test.lock")
		assert.True(t, acquired)

		// Second acquisition should fail
		acquired = coordinator.AcquireLock("test.lock")
		assert.False(t, acquired)

		// Check lock is held
		assert.True(t, coordinator.IsLockHeld("test.lock"))
	})

	t.Run("ReleaseLock", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryLockCoordinator()

		// Acquire lock
		acquired := coordinator.AcquireLock("test.lock")
		assert.True(t, acquired)
		assert.True(t, coordinator.IsLockHeld("test.lock"))

		// Release lock
		coordinator.ReleaseLock("test.lock")
		assert.False(t, coordinator.IsLockHeld("test.lock"))

		// Should be able to acquire again
		acquired = coordinator.AcquireLock("test.lock")
		assert.True(t, acquired)
	})

	t.Run("SubscribeToLock", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryLockCoordinator()

		received := make(chan agent.LockEventType, 10)
		cancel, err := coordinator.SubscribeToLock("test.lock", func(ctx context.Context, event agent.LockEventType) {
			received <- event
		})
		require.NoError(t, err)
		defer cancel()

		ctx := testutil.Context(t, testutil.WaitShort)

		// Acquire lock - should trigger acquired event
		coordinator.AcquireLock("test.lock")
		event := testutil.RequireReceive(ctx, t, received)
		assert.Equal(t, agent.LockEventTypeAcquired, event)

		// Release lock - should trigger released event
		coordinator.ReleaseLock("test.lock")
		event = testutil.RequireReceive(ctx, t, received)
		assert.Equal(t, agent.LockEventTypeReleased, event)
	})

	t.Run("SubscribeToLockAfterAcquire", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryLockCoordinator()

		// Acquire lock first
		coordinator.AcquireLock("test.lock")

		// Subscribe after acquisition - should receive historical event
		received := make(chan agent.LockEventType, 10)
		cancel, err := coordinator.SubscribeToLock("test.lock", func(ctx context.Context, event agent.LockEventType) {
			received <- event
		})
		require.NoError(t, err)
		defer cancel()

		ctx := testutil.Context(t, testutil.WaitShort)
		event := testutil.RequireReceive(ctx, t, received)
		assert.Equal(t, agent.LockEventTypeAcquired, event)
	})

	t.Run("GetLockHistory", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryLockCoordinator()

		// Initially no history
		history := coordinator.GetLockHistory("test.lock")
		assert.Empty(t, history)

		// Acquire lock
		coordinator.AcquireLock("test.lock")
		history = coordinator.GetLockHistory("test.lock")
		require.Len(t, history, 1)
		assert.Equal(t, agent.LockEventTypeAcquired, history[0].Type)

		// Release lock
		coordinator.ReleaseLock("test.lock")
		history = coordinator.GetLockHistory("test.lock")
		require.Len(t, history, 2)
		assert.Equal(t, agent.LockEventTypeAcquired, history[0].Type)
		assert.Equal(t, agent.LockEventTypeReleased, history[1].Type)

		// Acquire again
		coordinator.AcquireLock("test.lock")
		history = coordinator.GetLockHistory("test.lock")
		require.Len(t, history, 3)
		assert.Equal(t, agent.LockEventTypeAcquired, history[2].Type)
	})

	t.Run("MultipleLocks", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryLockCoordinator()

		// Acquire different locks
		assert.True(t, coordinator.AcquireLock("lock1"))
		assert.True(t, coordinator.AcquireLock("lock2"))
		assert.True(t, coordinator.AcquireLock("lock3"))

		// All should be held
		assert.True(t, coordinator.IsLockHeld("lock1"))
		assert.True(t, coordinator.IsLockHeld("lock2"))
		assert.True(t, coordinator.IsLockHeld("lock3"))

		// Release one
		coordinator.ReleaseLock("lock2")
		assert.True(t, coordinator.IsLockHeld("lock1"))
		assert.False(t, coordinator.IsLockHeld("lock2"))
		assert.True(t, coordinator.IsLockHeld("lock3"))
	})

	t.Run("ConcurrentAcquisition", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryLockCoordinator()

		// Simulate concurrent acquisition attempts
		results := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				results <- coordinator.AcquireLock("concurrent.lock")
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
		assert.True(t, coordinator.IsLockHeld("concurrent.lock"))
	})

	t.Run("Close", func(t *testing.T) {
		t.Parallel()

		coordinator := agent.NewMemoryLockCoordinator()

		// Acquire a lock
		coordinator.AcquireLock("test.lock")
		assert.True(t, coordinator.IsLockHeld("test.lock"))

		// Close coordinator
		err := coordinator.Close()
		assert.NoError(t, err)

		// Should not be able to acquire locks after close
		acquired := coordinator.AcquireLock("test.lock")
		assert.False(t, acquired)

		// Should not be able to subscribe after close
		_, err = coordinator.SubscribeToLock("test.lock", func(ctx context.Context, event agent.LockEventType) {})
		assert.Error(t, err)
	})
}
