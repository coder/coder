package workspacestats_test

import (
	"bytes"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestTracker(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	log := testutil.Logger(t)

	tickCh := make(chan time.Time)
	flushCh := make(chan int, 1)
	wut := workspacestats.NewTracker(mDB,
		workspacestats.TrackerWithLogger(log),
		workspacestats.TrackerWithTickFlush(tickCh, flushCh),
	)
	defer wut.Close()

	// 1. No marked workspaces should imply no flush.
	now := dbtime.Now()
	tickCh <- now
	count := <-flushCh
	require.Equal(t, 0, count, "expected zero flushes")

	// 2. One marked workspace should cause a flush.
	ids := []uuid.UUID{uuid.New()}
	now = dbtime.Now()
	wut.Add(ids[0])
	mDB.EXPECT().BatchUpdateWorkspaceLastUsedAt(gomock.Any(), database.BatchUpdateWorkspaceLastUsedAtParams{
		LastUsedAt: now,
		IDs:        ids,
	}).Times(1)
	tickCh <- now
	count = <-flushCh
	require.Equal(t, 1, count, "expected one flush with one id")

	// 3. Lots of marked workspaces should also cause a flush.
	for i := 0; i < 31; i++ {
		ids = append(ids, uuid.New())
	}

	// Sort ids so mDB know what to expect.
	sort.Slice(ids, func(i, j int) bool {
		return bytes.Compare(ids[i][:], ids[j][:]) < 0
	})

	now = dbtime.Now()
	mDB.EXPECT().BatchUpdateWorkspaceLastUsedAt(gomock.Any(), database.BatchUpdateWorkspaceLastUsedAtParams{
		LastUsedAt: now,
		IDs:        ids,
	})
	for _, id := range ids {
		wut.Add(id)
	}
	tickCh <- now
	count = <-flushCh
	require.Equal(t, len(ids), count, "incorrect number of ids flushed")

	// 4. Try to cause a race condition!
	now = dbtime.Now()
	// Difficult to know what to EXPECT here, so we won't check strictly here.
	mDB.EXPECT().BatchUpdateWorkspaceLastUsedAt(gomock.Any(), gomock.Any()).MinTimes(1).MaxTimes(len(ids))
	// Try to force a race condition.
	var wg sync.WaitGroup
	count = 0
	for i := 0; i < len(ids); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tickCh <- now
		}()
		wut.Add(ids[i])
	}

	for i := 0; i < len(ids); i++ {
		count += <-flushCh
	}

	wg.Wait()
	require.Equal(t, len(ids), count, "incorrect number of ids flushed")

	// 5. Closing multiple times should not be a problem.
	wut.Close()
	wut.Close()
}

// This test performs a more 'integration-style' test with multiple instances.
func TestTracker_MultipleInstances(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("this test only makes sense with postgres")
	}

	// Given we have two coderd instances connected to the same database
	var (
		ctx   = testutil.Context(t, testutil.WaitLong)
		db, _ = dbtestutil.NewDB(t)
		// real pubsub is not safe for concurrent use, and this test currently
		// does not depend on pubsub
		ps       = pubsub.NewInMemory()
		wuTickA  = make(chan time.Time)
		wuFlushA = make(chan int, 1)
		wuTickB  = make(chan time.Time)
		wuFlushB = make(chan int, 1)
		clientA  = coderdtest.New(t, &coderdtest.Options{
			WorkspaceUsageTrackerTick:  wuTickA,
			WorkspaceUsageTrackerFlush: wuFlushA,
			Database:                   db,
			Pubsub:                     ps,
		})
		clientB = coderdtest.New(t, &coderdtest.Options{
			WorkspaceUsageTrackerTick:  wuTickB,
			WorkspaceUsageTrackerFlush: wuFlushB,
			Database:                   db,
			Pubsub:                     ps,
		})
		owner = coderdtest.CreateFirstUser(t, clientA)
		now   = dbtime.Now()
	)

	clientB.SetSessionToken(clientA.SessionToken())

	// Create a number of workspaces
	numWorkspaces := 10
	w := make([]dbfake.WorkspaceResponse, numWorkspaces)
	for i := 0; i < numWorkspaces; i++ {
		wr := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        owner.UserID,
			OrganizationID: owner.OrganizationID,
			LastUsedAt:     now,
		}).WithAgent().Do()
		w[i] = wr
	}

	// Use client A to update LastUsedAt of the first three
	require.NoError(t, clientA.PostWorkspaceUsage(ctx, w[0].Workspace.ID))
	require.NoError(t, clientA.PostWorkspaceUsage(ctx, w[1].Workspace.ID))
	require.NoError(t, clientA.PostWorkspaceUsage(ctx, w[2].Workspace.ID))
	// Use client B to update LastUsedAt of the next three
	require.NoError(t, clientB.PostWorkspaceUsage(ctx, w[3].Workspace.ID))
	require.NoError(t, clientB.PostWorkspaceUsage(ctx, w[4].Workspace.ID))
	require.NoError(t, clientB.PostWorkspaceUsage(ctx, w[5].Workspace.ID))
	// The next two will have updated from both instances
	require.NoError(t, clientA.PostWorkspaceUsage(ctx, w[6].Workspace.ID))
	require.NoError(t, clientB.PostWorkspaceUsage(ctx, w[6].Workspace.ID))
	require.NoError(t, clientA.PostWorkspaceUsage(ctx, w[7].Workspace.ID))
	require.NoError(t, clientB.PostWorkspaceUsage(ctx, w[7].Workspace.ID))
	// The last two will not report any usage.

	// Tick both with different times and wait for both flushes to complete
	nowA := now.Add(time.Minute)
	nowB := now.Add(2 * time.Minute)
	var wg sync.WaitGroup
	var flushedA, flushedB int
	wg.Add(1)
	go func() {
		defer wg.Done()
		wuTickA <- nowA
		flushedA = <-wuFlushA
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		wuTickB <- nowB
		flushedB = <-wuFlushB
	}()
	wg.Wait()

	// We expect 5 flushed IDs each
	require.Equal(t, 5, flushedA)
	require.Equal(t, 5, flushedB)

	// Fetch updated workspaces
	updated := make([]codersdk.Workspace, numWorkspaces)
	for i := 0; i < numWorkspaces; i++ {
		ws, err := clientA.Workspace(ctx, w[i].Workspace.ID)
		require.NoError(t, err)
		updated[i] = ws
	}
	// We expect the first three to have the timestamp of flushA
	require.Equal(t, nowA.UTC(), updated[0].LastUsedAt.UTC())
	require.Equal(t, nowA.UTC(), updated[1].LastUsedAt.UTC())
	require.Equal(t, nowA.UTC(), updated[2].LastUsedAt.UTC())
	// We expect the next three to have the timestamp of flushB
	require.Equal(t, nowB.UTC(), updated[3].LastUsedAt.UTC())
	require.Equal(t, nowB.UTC(), updated[4].LastUsedAt.UTC())
	require.Equal(t, nowB.UTC(), updated[5].LastUsedAt.UTC())
	// The next two should have the timestamp of flushB as it is newer than flushA
	require.Equal(t, nowB.UTC(), updated[6].LastUsedAt.UTC())
	require.Equal(t, nowB.UTC(), updated[7].LastUsedAt.UTC())
	// And the last two should be untouched
	require.Equal(t, w[8].Workspace.LastUsedAt.UTC(), updated[8].LastUsedAt.UTC())
	require.Equal(t, w[8].Workspace.LastUsedAt.UTC(), updated[8].LastUsedAt.UTC())
	require.Equal(t, w[9].Workspace.LastUsedAt.UTC(), updated[9].LastUsedAt.UTC())
	require.Equal(t, w[9].Workspace.LastUsedAt.UTC(), updated[9].LastUsedAt.UTC())
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}
