package usage_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/enterprise/coderd/usage"
)

func TestPublishHealth(t *testing.T) {
	t.Parallel()

	firstFailure := time.Date(2026, time.August, 18, 1, 2, 3, 0, time.UTC)
	laterFailure := firstFailure.Add(time.Hour)
	publishedAt := firstFailure.Add(2 * time.Hour)

	health := &usage.PublishHealth{}
	health.RecordFailure(firstFailure)
	health.RecordFailure(laterFailure)
	require.Equal(t, firstFailure, health.Snapshot().FailureStartedAt)

	health.RecordPublished(publishedAt)
	health.RecordHealthy()
	snapshot := health.Snapshot()
	require.Equal(t, publishedAt, snapshot.LastPublishedAt)
	require.True(t, snapshot.FailureStartedAt.IsZero())

	health.RecordFailure(laterFailure)
	health.Reset()
	snapshot = health.Snapshot()
	require.True(t, snapshot.LastPublishedAt.IsZero())
	require.True(t, snapshot.FailureStartedAt.IsZero())
}

func TestPublishHealthConcurrentAccess(t *testing.T) {
	t.Parallel()

	health := &usage.PublishHealth{}
	now := time.Date(2026, time.August, 18, 1, 2, 3, 0, time.UTC)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 100 {
				health.RecordFailure(now)
				health.RecordPublished(now)
				health.RecordHealthy()
				_ = health.Snapshot()
			}
		})
	}
	wg.Wait()
}
