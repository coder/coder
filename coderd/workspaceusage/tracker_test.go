package workspaceusage_test

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/workspaceusage"
)

func TestTracker(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	log := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	tickCh := make(chan time.Time)
	flushCh := make(chan int, 1)
	wut := workspaceusage.New(mDB, workspaceusage.WithLogger(log), workspaceusage.WithTickChannel(tickCh), workspaceusage.WithFlushChannel(flushCh))
	t.Cleanup(wut.Close)

	go wut.Loop()

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
	for i := 0; i < 10; i++ {
		ids = append(ids, uuid.New())
	}

	// Sort ids so mockDB knows what to expect
	sort.Slice(ids, func(i, j int) bool {
		return strings.Compare(ids[i].String(), ids[j].String()) < 0
	})

	for _, id := range ids {
		wut.Add(id)
	}

	now = dbtime.Now()
	mDB.EXPECT().BatchUpdateWorkspaceLastUsedAt(gomock.Any(), database.BatchUpdateWorkspaceLastUsedAtParams{
		LastUsedAt: now,
		IDs:        ids,
	}).Times(1)
	tickCh <- now
	count = <-flushCh
	require.Equal(t, 11, count, "expected one flush with eleven ids")
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
