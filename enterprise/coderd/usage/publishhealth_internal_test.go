package usage

import (
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

	snapshot := health.Snapshot()
	require.True(t, snapshot.LastPublishedAt.IsZero())
	require.True(t, snapshot.FailureStartedAt.IsZero())
}
