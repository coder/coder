package provisionerdserver_test

import (
	"database/sql"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/testutil"
)

func TestAcquirer_WaitsOnNoJobs_RedisPubsub(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	redisServer := miniredis.RunT(t)
	ps, err := pubsub.NewRedis(ctx, testutil.Logger(t), "redis://"+redisServer.Addr())
	require.NoError(t, err)
	defer ps.Close()

	fs := newFakeOrderedStore()
	logger := testutil.Logger(t)
	uut := provisionerdserver.NewAcquirer(ctx, logger.Named("acquirer"), fs, ps)

	orgID := uuid.New()
	workerID := uuid.New()
	pt := []database.ProvisionerType{database.ProvisionerTypeEcho}
	tags := provisionerdserver.Tags{
		"environment": "on-prem",
	}
	acquiree := newTestAcquiree(t, orgID, workerID, pt, tags)
	jobID := uuid.New()
	err = fs.sendCtx(ctx, database.ProvisionerJob{}, sql.ErrNoRows)
	require.NoError(t, err)
	err = fs.sendCtx(ctx, database.ProvisionerJob{ID: jobID}, nil)
	require.NoError(t, err)
	acquiree.startAcquire(ctx, uut)
	require.Eventually(t, func() bool {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		return len(fs.params) == 1
	}, testutil.WaitShort, testutil.IntervalFast)
	acquiree.requireBlocked()

	postJob(t, ps, database.ProvisionerTypeEcho, provisionerdserver.Tags{
		"cool":   "tapes",
		"strong": "bad",
	})
	postJob(t, ps, database.ProvisionerTypeEcho, provisionerdserver.Tags{
		"environment": "fighters",
	})
	postJob(t, ps, database.ProvisionerTypeTerraform, provisionerdserver.Tags{
		"environment": "on-prem",
	})
	acquiree.requireBlocked()

	postJob(t, ps, database.ProvisionerTypeEcho, provisionerdserver.Tags{})
	job := acquiree.success(ctx)
	require.Equal(t, jobID, job.ID)
}
