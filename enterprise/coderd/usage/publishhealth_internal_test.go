package usage

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPublishHealthResetInvalidatesInflightCycle(t *testing.T) {
	t.Parallel()

	health := &PublishHealth{}
	epoch := health.currentEpoch()
	health.Reset()

	health.recordCycleFailure(epoch, time.Now())
	health.recordCyclePublished(epoch, time.Now())
	health.recordCycleHealthy(epoch)
	health.recordLocalDatabaseFailure(epoch, time.Now())

	snapshot := health.Snapshot()
	require.True(t, snapshot.LastPublishedAt.IsZero())
	require.True(t, snapshot.FailureStartedAt.IsZero())
	require.True(t, snapshot.LocalDatabaseFailureStartedAt.IsZero())
}

func TestPublishHealthRecordCycleHealthyClearsFailure(t *testing.T) {
	t.Parallel()

	health := &PublishHealth{}
	epoch := health.currentEpoch()
	failedAt := time.Date(2026, time.August, 18, 1, 2, 3, 0, time.UTC)
	publishedAt := failedAt.Add(time.Hour)

	health.recordCycleFailure(epoch, failedAt)
	health.recordCycleFailure(epoch, failedAt.Add(time.Hour))
	health.recordCyclePublished(epoch, publishedAt)
	require.Equal(t, failedAt, health.Snapshot().FailureStartedAt)
	require.Equal(t, publishedAt, health.Snapshot().LastPublishedAt)

	health.recordCycleHealthy(epoch)
	snapshot := health.Snapshot()
	require.Equal(t, publishedAt, snapshot.LastPublishedAt)
	require.True(t, snapshot.FailureStartedAt.IsZero())
}

func TestPublishHealthLocalDatabaseFailureSurvivesHealthyCycle(t *testing.T) {
	t.Parallel()

	health := &PublishHealth{}
	epoch := health.currentEpoch()
	failedAt := time.Date(2026, time.August, 18, 1, 2, 3, 0, time.UTC)
	publishedAt := failedAt.Add(time.Hour)

	health.recordLocalDatabaseFailure(epoch, failedAt)
	health.recordCycleFailure(epoch, failedAt)
	snapshot := health.Snapshot()
	require.Equal(t, failedAt, snapshot.FailureStartedAt)
	require.Equal(t, failedAt, snapshot.LocalDatabaseFailureStartedAt)

	// A later empty cycle is healthy as a Tallyman publish, but cannot prove
	// the local database recovered while claimed events are still held.
	health.recordCycleHealthy(epoch)
	snapshot = health.Snapshot()
	require.Equal(t, failedAt, snapshot.FailureStartedAt)
	require.Equal(t, failedAt, snapshot.LocalDatabaseFailureStartedAt)
	require.True(t, snapshot.LastPublishedAt.IsZero())

	health.recordLocalDatabaseSuccess(epoch)
	health.recordCyclePublished(epoch, publishedAt)
	health.recordCycleHealthy(epoch)
	snapshot = health.Snapshot()
	require.Equal(t, publishedAt, snapshot.LastPublishedAt)
	require.True(t, snapshot.FailureStartedAt.IsZero())
	require.True(t, snapshot.LocalDatabaseFailureStartedAt.IsZero())
}

func TestPublishHealthFailureStartedAtIsEarliest(t *testing.T) {
	t.Parallel()

	health := &PublishHealth{}
	epoch := health.currentEpoch()
	earlier := time.Date(2026, time.August, 18, 1, 2, 3, 0, time.UTC)
	later := earlier.Add(time.Hour)

	health.recordCycleFailure(epoch, earlier)
	health.recordLocalDatabaseFailure(epoch, later)
	require.Equal(t, earlier, health.Snapshot().FailureStartedAt)
	require.Equal(t, later, health.Snapshot().LocalDatabaseFailureStartedAt)

	health.Reset()
	epoch = health.currentEpoch()
	health.recordLocalDatabaseFailure(epoch, earlier)
	health.recordCycleFailure(epoch, later)
	require.Equal(t, earlier, health.Snapshot().FailureStartedAt)
	require.Equal(t, earlier, health.Snapshot().LocalDatabaseFailureStartedAt)
}

func TestPublishHealthResetClearsRecordedOutcomes(t *testing.T) {
	t.Parallel()

	health := &PublishHealth{}
	epoch := health.currentEpoch()
	now := time.Date(2026, time.August, 18, 1, 2, 3, 0, time.UTC)
	health.recordCycleFailure(epoch, now)
	health.recordLocalDatabaseFailure(epoch, now)
	health.recordCyclePublished(epoch, now.Add(time.Hour))

	health.Reset()
	snapshot := health.Snapshot()
	require.True(t, snapshot.LastPublishedAt.IsZero())
	require.True(t, snapshot.FailureStartedAt.IsZero())
	require.True(t, snapshot.LocalDatabaseFailureStartedAt.IsZero())
}

func TestPublishHealthConcurrentAccess(t *testing.T) {
	t.Parallel()

	health := &PublishHealth{}
	now := time.Date(2026, time.August, 18, 1, 2, 3, 0, time.UTC)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 100 {
				epoch := health.currentEpoch()
				health.recordCycleFailure(epoch, now)
				health.recordCyclePublished(epoch, now)
				health.recordCycleHealthy(epoch)
				health.recordLocalDatabaseFailure(epoch, now)
				health.recordLocalDatabaseSuccess(epoch)
				health.Reset()
				_ = health.Snapshot()
			}
		})
	}
	wg.Wait()
}
