package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/semaphore"
	"golang.org/x/xerrors"

	agplcoderd "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

// longLockTimeout keeps the lock timeout out of reach of driveBackoff, which
// advances the mock clock through every backoff.
const longLockTimeout = 24 * time.Hour

func TestQuotaAdmissionLimit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		maxOpen    int
		configured int64
		want       int64
		wantErr    bool
	}{
		{name: "Automatic", maxOpen: 10, want: 2},
		{name: "AutomaticSingleConnection", maxOpen: 1, want: 1},
		{name: "ExplicitSingleConnection", maxOpen: 1, configured: 1, want: 1},
		{name: "TooManyForSingleConnection", maxOpen: 1, configured: 2, wantErr: true},
		{name: "Override", maxOpen: 10, configured: 9, want: 9},
		{name: "WholePool", maxOpen: 10, configured: 10, wantErr: true},
		{name: "Negative", maxOpen: 10, configured: -1, wantErr: true},
		{name: "AutomaticCap", maxOpen: 40, want: 8},
		{name: "UnlimitedPoolAutomatic", want: 8},
		{name: "UnlimitedPoolOverride", configured: 50, want: 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			limit, err := quotaAdmissionLimit(tc.maxOpen, tc.configured)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, limit)
		})
	}
}

func TestNewRejectsQuotaAdmissionWholePool(t *testing.T) {
	t.Parallel()
	api, err := New(context.Background(), &Options{Options: &agplcoderd.Options{
		DeploymentValues: &codersdk.DeploymentValues{
			PostgresConnMaxOpen:       serpent.Int64(10),
			QuotaMaxConcurrentCommits: serpent.Int64(10),
		},
	}})
	require.Nil(t, api)
	require.ErrorContains(t, err, "invalid quota max concurrent commits")
	require.ErrorContains(t, err, "must be less than postgres max open connections (10)")
}

func TestCommitQuotaConcurrency(t *testing.T) {
	t.Parallel()

	// Two cost-bearing builds for one owner and organization, with budget
	// for only one. The second commit must back off while the first holds
	// the lock, then see its cost and deny.
	t.Run("SameOwnerAndOrgTakeTurns", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 10, user)
		first := f.build(t, user, org)
		second := f.build(t, user, org)

		pause := newQuotaPause()
		firstDone := commitAsync(ctx, f.committer(t, pause.store(f.db)), first, 10)
		testutil.TryReceive(ctx, t, pause.paused)

		clock := quartz.NewMock(t)
		backoff := clock.Trap().NewTimer("commitQuota", "backoff")
		defer backoff.Close()
		waiter := f.committer(t, f.db)
		waiter.clock = clock
		secondDone := commitAsync(ctx, waiter, second, 10)
		call := backoff.MustWait(ctx)
		require.Zero(t, f.advisoryLocks(ctx, t, true /* waitingOnly */), "a miss must not wait in PostgreSQL")

		close(pause.resume)
		firstRes := testutil.RequireReceive(ctx, t, firstDone)
		require.NoError(t, firstRes.err)
		require.True(t, firstRes.resp.Ok)

		call.MustRelease(ctx)
		clock.Advance(call.Duration).MustWait(ctx)
		secondRes := testutil.RequireReceive(ctx, t, secondDone)
		require.NoError(t, secondRes.err)
		require.False(t, secondRes.resp.Ok, "second commit should see the first commit's cost")
		require.EqualValues(t, 10, secondRes.resp.CreditsConsumed)
		require.EqualValues(t, 10, secondRes.resp.Budget)

		require.EqualValues(t, 10, f.consumed(ctx, t, user, org))
	})

	t.Run("BurstRespectsBudget", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 20, user)
		clock := quartz.NewMock(t)
		driveBackoff(ctx, t, clock)

		const builds = 6
		results := make([]<-chan commitResult, 0, builds)
		for range builds {
			c := f.committer(t, f.db)
			c.clock, c.lockTimeout = clock, longLockTimeout
			results = append(results, commitAsync(ctx, c, f.build(t, user, org), 10))
		}

		var permitted int
		for _, done := range results {
			res := testutil.RequireReceive(ctx, t, done)
			require.NoError(t, res.err)
			if res.resp.Ok {
				permitted++
			}
		}
		require.Equal(t, 2, permitted)
		require.EqualValues(t, 20, f.consumed(ctx, t, user, org))
	})

	// While another replica holds the lock, more same-key commits than the
	// pool has connections keep retrying, and unrelated queries on the pool
	// still get a connection. The commits then see the holder's cost.
	t.Run("HeldKeyLeavesPoolUsable", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 30, user)
		holder := f.holdQuotaLock(ctx, t, user, org)
		_, err := holder.ExecContext(ctx, "UPDATE workspace_builds SET daily_cost = 10 WHERE id = $1", f.build(t, user, org).ID)
		require.NoError(t, err)

		const poolSize = 2
		quotaDB := f.pool(t, poolSize)
		limit, err := quotaAdmissionLimit(poolSize, 0)
		require.NoError(t, err)
		admission := semaphore.NewWeighted(limit)
		clock := quartz.NewMock(t)
		driveBackoff(ctx, t, clock)
		var tryLocks atomic.Int32
		results := make([]<-chan commitResult, 0, 2*poolSize)
		for range 2 * poolSize {
			c := f.committer(t, &quotaHookStore{Store: database.New(quotaDB), tryLocks: &tryLocks})
			c.admission, c.clock, c.lockTimeout = admission, clock, longLockTimeout
			results = append(results, commitAsync(ctx, c, f.build(t, user, org), 10))
		}
		testutil.Eventually(ctx, t, func(context.Context) bool {
			return tryLocks.Load() >= 20
		}, testutil.IntervalFast)
		for range 20 {
			require.LessOrEqual(t, quotaDB.Stats().InUse, 1, "quota must stay within its admission limit")
			queryCtx, cancel := context.WithTimeout(ctx, testutil.WaitShort)
			_, err := quotaDB.ExecContext(queryCtx, "SELECT 1")
			cancel()
			require.NoError(t, err, "unrelated query must get a connection")
		}

		require.NoError(t, holder.Commit())
		var permitted int
		for _, done := range results {
			res := testutil.RequireReceive(ctx, t, done)
			require.NoError(t, res.err)
			if res.resp.Ok {
				permitted++
			}
		}
		require.Equal(t, 2, permitted)
		require.EqualValues(t, 30, f.consumed(ctx, t, user, org))
	})

	// A commit that holds the lock for one owner and organization must not
	// block commits for other owners or other organizations.
	t.Run("OtherScopesDoNotWait", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		otherUser := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 10, user, otherUser)
		otherOrg := f.org(t, 10, user)

		pause := newQuotaPause()
		holderDone := commitAsync(ctx, f.committer(t, pause.store(f.db)), f.build(t, user, org), 10)
		testutil.TryReceive(ctx, t, pause.paused)

		otherOwnerRes := testutil.RequireReceive(ctx, t,
			commitAsync(ctx, f.committer(t, f.db), f.build(t, otherUser, org), 10))
		require.NoError(t, otherOwnerRes.err)
		require.True(t, otherOwnerRes.resp.Ok)

		otherOrgRes := testutil.RequireReceive(ctx, t,
			commitAsync(ctx, f.committer(t, f.db), f.build(t, user, otherOrg), 10))
		require.NoError(t, otherOrgRes.err)
		require.True(t, otherOrgRes.resp.Ok)

		close(pause.resume)
		held := testutil.RequireReceive(ctx, t, holderDone)
		require.NoError(t, held.err)
		require.True(t, held.resp.Ok)
	})

	// Slow quota work stays admitted while indexed lookups proceed without
	// taking a slot. The remaining connections must remain available.
	t.Run("SlowQuotaLeavesLookupAndPoolUsable", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		users := make([]database.User, 4)
		for i := range users {
			users[i] = dbgen.User(t, f.db, database.User{})
		}
		org := f.org(t, 10, users...)
		builds := make([]database.WorkspaceBuild, len(users))
		for i, user := range users {
			builds[i] = f.build(t, user, org)
		}
		quotaDB := f.pool(t, 4)
		limit, err := quotaAdmissionLimit(4, 0)
		require.NoError(t, err)
		admission := semaphore.NewWeighted(limit)
		entered, release := make(chan struct{}, len(users)), make(chan struct{})
		var lookups atomic.Int32
		store := &quotaHookStore{
			Store: database.New(quotaDB),
			afterLookup: func(context.Context) error {
				lookups.Add(1)
				return nil
			},
			beforeConsumed: func(ctx context.Context) error {
				entered <- struct{}{}
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		}
		holder := f.committer(t, store)
		holder.admission = admission
		holderDone := commitAsync(ctx, holder, builds[0], 10)
		testutil.TryReceive(ctx, t, entered)

		clock := quartz.NewMock(t)
		admit := clock.Trap().AfterFunc("commitQuota", "admission")
		defer admit.Close()
		results := make([]<-chan commitResult, 0, len(users)-1)
		for _, build := range builds[1:] {
			c := f.committer(t, store)
			c.admission, c.clock = admission, clock
			results = append(results, commitAsync(ctx, c, build, 10))
			admit.MustWait(ctx).MustRelease(ctx)
		}
		testutil.Eventually(ctx, t, func(context.Context) bool {
			return int(lookups.Load()) == len(users)
		}, testutil.IntervalFast)

		spare := make([]*sql.Conn, 0, 3)
		for range 3 {
			checkoutCtx, cancel := context.WithTimeout(ctx, testutil.WaitShort)
			conn, err := quotaDB.Conn(checkoutCtx)
			cancel()
			require.NoError(t, err, "slow quota work must leave spare pool capacity")
			t.Cleanup(func() { _ = conn.Close() })
			spare = append(spare, conn)
		}
		clock.Advance(quotaLockTimeout).MustWait(ctx)
		for _, done := range results {
			res := testutil.RequireReceive(ctx, t, done)
			require.ErrorContains(t, res.err, "timed out after 30s waiting for workspace quota lock")
			require.NotErrorIs(t, res.err, context.Canceled)
		}

		for _, conn := range spare {
			require.NoError(t, conn.Close())
		}
		close(release)
		res := testutil.RequireReceive(ctx, t, holderDone)
		require.NoError(t, res.err)
		require.True(t, res.resp.Ok)
	})
	// If InTx runs the closure more than once, as it does for SERIALIZABLE
	// transactions, the response must come from the attempt that committed,
	// not from an earlier attempt that was rolled back after permitting the
	// build.
	t.Run("RetryUsesFinalAttempt", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 10, user)
		first := f.build(t, user, org)
		second := f.build(t, user, org)

		store := &retryOnceStore{
			Store: f.db,
			betweenAttempts: func() {
				// The first attempt's cost was rolled back, so this build
				// takes the whole budget.
				res, err := f.committer(t, f.db).CommitQuota(ctx, commitRequest(second, 10))
				assert.NoError(t, err)
				assert.True(t, res.GetOk())
			},
		}
		res, err := f.committer(t, store).CommitQuota(ctx, commitRequest(first, 10))
		require.NoError(t, err)
		require.False(t, res.Ok, "response must match the attempt that committed")
		require.EqualValues(t, 10, res.CreditsConsumed)

		require.EqualValues(t, 10, f.consumed(ctx, t, user, org))
		require.EqualValues(t, 0, f.buildCost(ctx, t, first))
	})

	// A replica that does not take the lock must still participate in SSI,
	// so old and new quota commits cannot both spend the same budget.
	t.Run("UnlockedSerializableCommitCannotOverspend", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 10, user)
		first := f.build(t, user, org)
		second := f.build(t, user, org)

		oldStore := database.New(f.sqlDB, database.WithSerialRetryCount(1))
		old := dbtestutil.StartTx(t, oldStore, &database.TxOptions{Isolation: sql.LevelSerializable})
		finishOld := sync.OnceValue(old.Done)
		t.Cleanup(func() { _ = finishOld() })
		consumed, err := old.GetQuotaConsumedForUser(ctx, database.GetQuotaConsumedForUserParams{
			OwnerID: user.ID, OrganizationID: org.ID,
		})
		require.NoError(t, err)
		require.Zero(t, consumed)
		budget, err := old.GetQuotaAllowanceForUser(ctx, database.GetQuotaAllowanceForUserParams{
			UserID: user.ID, OrganizationID: org.ID,
		})
		require.NoError(t, err)
		require.EqualValues(t, 10, budget)

		res, err := f.committer(t, f.db).CommitQuota(ctx, commitRequest(second, 10))
		require.NoError(t, err)
		require.True(t, res.Ok)
		err = old.UpdateWorkspaceBuildCostByID(ctx, database.UpdateWorkspaceBuildCostByIDParams{
			ID: first.ID, DailyCost: 10,
		})
		commitErr := finishOld()
		require.True(t, database.IsSerializedError(err) || database.IsSerializedError(commitErr),
			"the unlocked commit must abort instead of overspending: update=%v commit=%v", err, commitErr)
		require.EqualValues(t, 0, f.buildCost(ctx, t, first))
		require.EqualValues(t, 10, f.consumed(ctx, t, user, org))
	})

	// The try-lock can acquire the lock after its snapshot was taken and a
	// holder committed. SSI must reject the stale cost increase and retry.
	t.Run("SnapshotBeforeLockReleaseRetries", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 10, user)
		first := f.build(t, user, org)
		second := f.build(t, user, org)

		holderPause := newQuotaPause()
		holderDone := commitAsync(ctx, f.committer(t, holderPause.store(f.db)), first, 10)
		testutil.TryReceive(ctx, t, holderPause.paused)

		snapshotPause := newQuotaPause()
		var attempts, serialized atomic.Int32
		store := &quotaHookStore{
			Store:     database.New(f.sqlDB),
			onAttempt: func() { attempts.Add(1) },
			afterAttempt: func(err error) {
				if database.IsSerializedError(err) {
					serialized.Add(1)
				}
			},
			beforeTryLock: func(ctx context.Context, tx database.Store) error {
				// This table-free statement pins the snapshot without
				// creating extra read dependencies.
				if _, err := tx.GetDatabaseNow(ctx); err != nil {
					return err
				}
				return snapshotPause.wait(ctx)
			},
		}
		waiterDone := commitAsync(ctx, f.committer(t, store), second, 10)
		testutil.TryReceive(ctx, t, snapshotPause.paused)
		close(holderPause.resume)
		holderRes := testutil.RequireReceive(ctx, t, holderDone)
		require.NoError(t, holderRes.err)
		require.True(t, holderRes.resp.Ok)
		close(snapshotPause.resume)

		res := testutil.RequireReceive(ctx, t, waiterDone)
		require.NoError(t, res.err)
		require.False(t, res.resp.Ok)
		require.EqualValues(t, 10, res.resp.CreditsConsumed)
		require.EqualValues(t, 10, res.resp.Budget)
		require.EqualValues(t, 2, attempts.Load())
		require.EqualValues(t, 1, serialized.Load())
		require.EqualValues(t, 0, f.buildCost(ctx, t, second))
		require.EqualValues(t, 10, f.consumed(ctx, t, user, org))
	})

	// InTx's immediate retries can be exhausted by SSI conflicts across
	// owners. The outer loop must release admission and back off safely.
	t.Run("SerializationFailuresBackOffAndRetry", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 10, user)
		build := f.build(t, user, org)
		quotaDB := f.pool(t, 1)
		var attempts atomic.Int32
		store := &quotaHookStore{
			Store: database.New(quotaDB),
			beforeConsumed: func(context.Context) error {
				if attempts.Add(1) <= 7 {
					return &pq.Error{Code: "40001"}
				}
				return nil
			},
		}
		clock := quartz.NewMock(t)
		backoff := clock.Trap().NewTimer("commitQuota", "backoff")
		defer backoff.Close()
		c := f.committer(t, store)
		c.clock = clock
		c.admission = semaphore.NewWeighted(1)
		done := commitAsync(ctx, c, build, 10)

		for range 2 {
			call := backoff.MustWait(ctx)
			require.Zero(t, quotaDB.Stats().InUse)
			require.True(t, c.admission.TryAcquire(1), "backoff must release admission")
			c.admission.Release(1)
			call.MustRelease(ctx)
			clock.Advance(call.Duration).MustWait(ctx)
		}
		res := testutil.RequireReceive(ctx, t, done)
		require.NoError(t, res.err)
		require.True(t, res.resp.Ok)
		require.EqualValues(t, 8, attempts.Load())
		require.EqualValues(t, 10, f.buildCost(ctx, t, build))
		require.EqualValues(t, 10, f.consumed(ctx, t, user, org))
	})

	t.Run("SerializationFailuresUseWaitBudget", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 10, user)
		build := f.build(t, user, org)
		var attempts atomic.Int32
		store := &quotaHookStore{
			Store: database.New(f.sqlDB),
			beforeConsumed: func(context.Context) error {
				attempts.Add(1)
				return &pq.Error{Code: "40001"}
			},
		}
		clock := quartz.NewMock(t)
		driveBackoff(ctx, t, clock)
		c := f.committer(t, store)
		c.clock = clock
		res := testutil.RequireReceive(ctx, t, commitAsync(ctx, c, build, 10))
		require.ErrorContains(t, res.err, "timed out after 30s waiting for workspace quota lock")
		require.NotErrorIs(t, res.err, context.Canceled)
		require.False(t, database.IsSerializedError(res.err))
		require.Greater(t, attempts.Load(), int32(3))
		require.EqualValues(t, 0, f.buildCost(ctx, t, build))
		require.Zero(t, f.advisoryLocks(ctx, t, false /* waitingOnly */))
	})

	t.Run("ErrorReleasesLock", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 10, user)

		errInjected := xerrors.New("injected")
		failing := &quotaHookStore{Store: f.db, beforeConsumed: func(context.Context) error {
			return errInjected
		}}
		_, err := f.committer(t, failing).CommitQuota(ctx, commitRequest(f.build(t, user, org), 10))
		require.ErrorIs(t, err, errInjected)
		require.Zero(t, f.advisoryLocks(ctx, t, false /* waitingOnly */))

		res, err := f.committer(t, f.db).CommitQuota(ctx, commitRequest(f.build(t, user, org), 10))
		require.NoError(t, err)
		require.True(t, res.Ok)
	})

	// Unrelated failures must not be retried. A retry here would wait on a
	// mock clock that nothing advances.
	t.Run("DoesNotRetryFailedAttempts", func(t *testing.T) {
		t.Parallel()

		errInjected := xerrors.New("injected")
		for _, tc := range []struct {
			name string
			held bool
			hook func(*quotaHookStore)
			// cost is what a commit that fails after writing it leaves.
			cost int32
		}{
			{name: "TryLockFails", hook: func(s *quotaHookStore) { s.tryLockErr = errInjected }},
			{name: "MissCommitFails", held: true, hook: func(s *quotaHookStore) { s.attemptErr = errInjected }},
			{name: "QueryFailsAfterLock", hook: func(s *quotaHookStore) {
				s.beforeConsumed = func(context.Context) error { return errInjected }
			}},
			{name: "CommitOutcomeUnknown", hook: func(s *quotaHookStore) { s.attemptErr = errInjected }, cost: 10},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				f := newQuotaFixture(t)
				user := dbgen.User(t, f.db, database.User{})
				org := f.org(t, 10, user)
				if tc.held {
					f.holdQuotaLock(ctx, t, user, org)
				}
				var tryLocks atomic.Int32
				store := &quotaHookStore{Store: f.db, tryLocks: &tryLocks}
				tc.hook(store)
				c := f.committer(t, store)
				c.clock = quartz.NewMock(t)
				build := f.build(t, user, org)

				res := testutil.RequireReceive(ctx, t, commitAsync(ctx, c, build, 10))
				require.ErrorIs(t, res.err, errInjected)
				require.EqualValues(t, 1, tryLocks.Load())
				require.Equal(t, tc.cost, f.buildCost(ctx, t, build))
			})
		}
	})

	// The holder's own timeout passes while it holds the lock, which must
	// not affect the rest of its transaction.
	t.Run("LockWaitTimesOut", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		// Budget for both builds, so the waiter fails only because of the
		// timeout.
		org := f.org(t, 20, user)

		clock := quartz.NewMock(t)
		driveBackoff(ctx, t, clock)
		holderClock := quartz.NewMock(t)
		pause := newQuotaPause()
		holder := f.committer(t, pause.store(f.db))
		holder.clock, holder.lockTimeout = holderClock, testutil.IntervalMedium
		holderDone := commitAsync(ctx, holder, f.build(t, user, org), 10)
		testutil.TryReceive(ctx, t, pause.paused)

		waiterDB, err := sql.Open("postgres", f.url)
		require.NoError(t, err)
		t.Cleanup(func() { _ = waiterDB.Close() })
		waiterDB.SetMaxOpenConns(1)
		var beforePID int
		require.NoError(t, waiterDB.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&beforePID))
		_, err = waiterDB.ExecContext(ctx, "SET lock_timeout = '7s'")
		require.NoError(t, err)

		waiter := f.committer(t, database.New(waiterDB))
		waiter.clock, waiter.lockTimeout = clock, testutil.IntervalSlow
		waiterBuild := f.build(t, user, org)
		res := testutil.RequireReceive(ctx, t, commitAsync(ctx, waiter, waiterBuild, 10))
		require.ErrorContains(t, res.err,
			fmt.Sprintf("timed out after %s waiting for workspace quota lock; quota checks are taking too long, please retry the build", testutil.IntervalSlow))
		require.NotErrorIs(t, res.err, context.Canceled, "the RPC wasn't canceled")

		var afterPID int
		var lockTimeout string
		require.NoError(t, waiterDB.QueryRowContext(ctx, "SELECT pg_backend_pid(), current_setting('lock_timeout')").Scan(&afterPID, &lockTimeout))
		require.Equal(t, beforePID, afterPID, "misses must keep the pooled connection")
		require.Equal(t, "7s", lockTimeout, "misses must preserve the session setting")
		require.EqualValues(t, 0, f.buildCost(ctx, t, waiterBuild))

		holderClock.Advance(testutil.IntervalMedium).MustWait(ctx)
		close(pause.resume)
		held := testutil.RequireReceive(ctx, t, holderDone)
		require.NoError(t, held.err)
		require.True(t, held.resp.Ok)
		require.EqualValues(t, 10, f.consumed(ctx, t, user, org))
		require.Zero(t, f.advisoryLocks(ctx, t, false /* waitingOnly */))
	})

	// The lock timeout and cancellation both end a wait for a connection.
	t.Run("CheckoutObservesTimeout", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		build := f.build(t, user, f.org(t, 10, user))
		quotaDB := f.pool(t, 1)
		clock := quartz.NewMock(t)
		admit := clock.Trap().AfterFunc("commitQuota", "admission")
		defer admit.Close()
		store := &quotaHookStore{Store: database.New(quotaDB)}
		c := f.committer(t, store)
		c.clock = clock

		for _, timeout := range []bool{true, false} {
			commitCtx, cancel := context.WithCancel(ctx)
			pause := newQuotaPause()
			store.afterLookup = pause.wait
			done := commitAsync(commitCtx, c, build, 10)
			admit.MustWait(ctx).MustRelease(ctx)
			testutil.TryReceive(ctx, t, pause.paused)
			// Drain the pool after lookup to exercise the quota transaction's
			// bounded checkout, rather than the RPC-context lookup.
			held, err := quotaDB.Conn(ctx)
			require.NoError(t, err)
			t.Cleanup(func() { _ = held.Close() })
			close(pause.resume)
			wantWaits := int64(1)
			if !timeout {
				wantWaits = 2
			}
			testutil.Eventually(ctx, t, func(context.Context) bool {
				return quotaDB.Stats().WaitCount == wantWaits
			}, testutil.IntervalFast)
			if timeout {
				clock.Advance(quotaLockTimeout).MustWait(ctx)
				require.ErrorContains(t, testutil.RequireReceive(ctx, t, done).err, "timed out after 30s waiting for workspace quota lock")
			} else {
				cancel()
				require.ErrorIs(t, testutil.RequireReceive(ctx, t, done).err, context.Canceled)
			}
			cancel()
			require.NoError(t, held.Close())
		}
	})

	// Within a transaction, a commit would back off on the outer
	// transaction's connection and could read its snapshot.
	t.Run("RejectsNestedTransaction", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		build := f.build(t, user, f.org(t, 10, user))
		err := f.db.InTx(func(tx database.Store) error {
			_, err := f.committer(t, tx).CommitQuota(ctx, commitRequest(build, 10))
			require.ErrorIs(t, err, database.ErrNestedTransaction)
			_, err = tx.GetWorkspaceBuildByID(ctx, build.ID)
			return err
		}, nil)
		require.NoError(t, err, "the outer transaction must remain usable")
		require.EqualValues(t, 0, f.buildCost(ctx, t, build))
	})

	t.Run("LaterLockTimeoutIsNotQuotaLockTimeout", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 10, user)
		build := f.build(t, user, org)
		holder, err := f.sqlDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = holder.Rollback() })
		_, err = holder.ExecContext(ctx, "SELECT 1 FROM workspace_builds WHERE id = $1 FOR UPDATE", build.ID)
		require.NoError(t, err)

		quotaDB, err := sql.Open("postgres", f.url)
		require.NoError(t, err)
		t.Cleanup(func() { _ = quotaDB.Close() })
		quotaDB.SetMaxOpenConns(1)
		_, err = quotaDB.ExecContext(ctx, "SET lock_timeout = '250ms'")
		require.NoError(t, err)
		_, err = f.committer(t, database.New(quotaDB)).CommitQuota(ctx, commitRequest(build, 10))
		pgErr, ok := errors.AsType[*pq.Error](err)
		require.True(t, ok)
		require.Equal(t, pq.ErrorCode("55P03"), pgErr.Code)
		require.NotContains(t, err.Error(), "workspace quota lock")
		require.EqualValues(t, 0, f.buildCost(ctx, t, build))
		require.Zero(t, f.advisoryLocks(ctx, t, false /* waitingOnly */))
	})

	t.Run("SameBuildDeadlineUpdateRetries", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 10, user)
		build := f.build(t, user, org)
		pause := newQuotaPause()
		var attempts atomic.Int32
		store := &quotaHookStore{
			Store:         database.New(f.sqlDB, database.WithSerialRetryCount(3)),
			onAttempt:     func() { attempts.Add(1) },
			afterConsumed: pause.wait,
		}
		done := commitAsync(ctx, f.committer(t, store), build, 10)
		testutil.TryReceive(ctx, t, pause.paused)

		deadline := dbtestutil.NowInDefaultTimezone().Add(time.Hour)
		maxDeadline := deadline.Add(time.Hour)
		err := f.db.InTx(func(tx database.Store) error {
			return tx.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
				ID:          build.ID,
				Deadline:    deadline,
				MaxDeadline: maxDeadline,
				UpdatedAt:   dbtestutil.NowInDefaultTimezone(),
			})
		}, nil)
		require.NoError(t, err)
		close(pause.resume)
		res := testutil.RequireReceive(ctx, t, done)
		require.NoError(t, res.err)
		require.True(t, res.resp.Ok)
		require.EqualValues(t, 10, res.resp.CreditsConsumed)
		require.EqualValues(t, 10, res.resp.Budget)
		require.EqualValues(t, 2, attempts.Load(), "the concurrent update must cause a safe serialization retry")
		got, err := f.db.GetWorkspaceBuildByID(ctx, build.ID)
		require.NoError(t, err)
		require.True(t, deadline.Equal(got.Deadline), "quota commit must preserve the concurrent deadline update")
		require.True(t, maxDeadline.Equal(got.MaxDeadline))
		require.EqualValues(t, 10, got.DailyCost)
		require.EqualValues(t, 10, f.consumed(ctx, t, user, org))
	})

	t.Run("CancelReleasesLock", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 10, user)

		holderCtx, cancelHolder := context.WithCancel(ctx)
		defer cancelHolder()
		pause := newQuotaPause()
		holderDone := commitAsync(holderCtx, f.committer(t, pause.store(f.db)), f.build(t, user, org), 10)
		testutil.TryReceive(ctx, t, pause.paused)

		// A commit canceled while it backs off gives up. Before backing off,
		// its miss gave back its connection and admission slot.
		waiterCtx, cancelWaiter := context.WithCancel(ctx)
		defer cancelWaiter()
		clock := quartz.NewMock(t)
		backoff := clock.Trap().NewTimer("commitQuota", "backoff")
		defer backoff.Close()
		waiterDB := f.pool(t, 1)
		waiter := f.committer(t, database.New(waiterDB))
		waiter.admission, waiter.clock = semaphore.NewWeighted(1), clock
		waiterDone := commitAsync(waiterCtx, waiter, f.build(t, user, org), 10)
		backoff.MustWait(ctx).MustRelease(ctx)
		require.Zero(t, waiterDB.Stats().InUse)
		require.True(t, waiter.admission.TryAcquire(1), "the admission slot must be free")
		waiter.admission.Release(1)
		cancelWaiter()
		require.ErrorIs(t, testutil.RequireReceive(ctx, t, waiterDone).err, context.Canceled)

		// A commit canceled while it holds the lock rolls back and
		// releases it.
		cancelHolder()
		close(pause.resume)
		require.Error(t, testutil.RequireReceive(ctx, t, holderDone).err)
		require.Zero(t, f.advisoryLocks(ctx, t, false /* waitingOnly */))

		res, err := f.committer(t, f.db).CommitQuota(ctx, commitRequest(f.build(t, user, org), 10))
		require.NoError(t, err)
		require.True(t, res.Ok)
		require.EqualValues(t, 10, f.consumed(ctx, t, user, org))
	})
}

type quotaFixture struct {
	db    database.Store
	sqlDB *sql.DB
	url   string
}

func newQuotaFixture(t *testing.T) quotaFixture {
	t.Helper()
	url := os.Getenv("CODER_PG_CONNECTION_URL")
	if url == "" {
		var err error
		url, err = dbtestutil.Open(t)
		require.NoError(t, err)
	}
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithURL(url))
	return quotaFixture{db: db, sqlDB: sqlDB, url: url}
}

func (f quotaFixture) org(t *testing.T, allowance int, members ...database.User) database.Organization {
	t.Helper()
	return dbfake.Organization(t, f.db).EveryoneAllowance(allowance).Members(members...).Do().Org
}

func (f quotaFixture) build(t *testing.T, owner database.User, org database.Organization) database.WorkspaceBuild {
	t.Helper()
	return dbfake.WorkspaceBuild(t, f.db, database.WorkspaceTable{
		OwnerID:        owner.ID,
		OrganizationID: org.ID,
	}).Do().Build
}

func (quotaFixture) committer(t *testing.T, store database.Store) *committer {
	t.Helper()
	limit, err := quotaAdmissionLimit(0, 0)
	require.NoError(t, err)
	return &committer{
		Log:       testutil.Logger(t),
		Database:  store,
		admission: semaphore.NewWeighted(limit),
		clock:     quartz.NewReal(),
	}
}

// pool opens a separate connection pool to the test database.
func (f quotaFixture) pool(t *testing.T, maxOpenConns int) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", f.url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(maxOpenConns)
	return db
}

// holdQuotaLock takes the quota lock in a transaction on its own pool, the
// way a commit on another replica would.
func (f quotaFixture) holdQuotaLock(ctx context.Context, t *testing.T, owner database.User, org database.Organization) *sql.Tx {
	t.Helper()
	tx, err := f.pool(t, 1).BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", database.WorkspaceQuotaLockID(owner.ID, org.ID))
	require.NoError(t, err)
	return tx
}

func (f quotaFixture) consumed(ctx context.Context, t *testing.T, owner database.User, org database.Organization) int64 {
	t.Helper()
	consumed, err := f.db.GetQuotaConsumedForUser(ctx, database.GetQuotaConsumedForUserParams{
		OwnerID:        owner.ID,
		OrganizationID: org.ID,
	})
	require.NoError(t, err)
	return consumed
}

func (f quotaFixture) buildCost(ctx context.Context, t *testing.T, build database.WorkspaceBuild) int32 {
	t.Helper()
	got, err := f.db.GetWorkspaceBuildByID(ctx, build.ID)
	require.NoError(t, err)
	return got.DailyCost
}

// advisoryLocks counts advisory locks in this test's database. Each subtest
// has its own database, so these are the quota locks. With waitingOnly, it
// counts only commits that are waiting for a lock.
func (f quotaFixture) advisoryLocks(ctx context.Context, t *testing.T, waitingOnly bool) int {
	t.Helper()
	var n int
	err := f.sqlDB.QueryRowContext(ctx, `
		SELECT count(*) FROM pg_locks
		WHERE locktype = 'advisory'
		AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
		AND (NOT $1 OR NOT granted)`, waitingOnly,
	).Scan(&n)
	require.NoError(t, err)
	return n
}

// driveBackoff fires each backoff timer on clock as soon as a commit sets it,
// so contended commits retry without waiting on real time. It assumes the
// clock's other timers come later, such as a longLockTimeout.
func driveBackoff(ctx context.Context, t *testing.T, clock *quartz.Mock) {
	trap := clock.Trap().NewTimer("commitQuota", "backoff")
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			call, err := trap.Wait(ctx)
			if err != nil || call.Release(ctx) != nil {
				return
			}
			if _, w := clock.AdvanceNext(); w.Wait(ctx) != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		trap.Close()
	})
}

type commitResult struct {
	resp *proto.CommitQuotaResponse
	err  error
}

func commitRequest(build database.WorkspaceBuild, cost int32) *proto.CommitQuotaRequest {
	return &proto.CommitQuotaRequest{JobId: build.JobID.String(), DailyCost: cost}
}

func commitAsync(ctx context.Context, c *committer, build database.WorkspaceBuild, cost int32) <-chan commitResult {
	done := make(chan commitResult, 1)
	go func() {
		resp, err := c.CommitQuota(ctx, commitRequest(build, cost))
		done <- commitResult{resp: resp, err: err}
	}()
	return done
}

// quotaPause pauses the first CommitQuota attempt at its configured hook.
type quotaPause struct {
	started atomic.Bool
	paused  chan struct{}
	resume  chan struct{}
}

func newQuotaPause() *quotaPause {
	return &quotaPause{
		paused: make(chan struct{}),
		resume: make(chan struct{}),
	}
}

func (p *quotaPause) store(db database.Store) database.Store {
	return &quotaHookStore{Store: db, beforeConsumed: p.wait}
}

// wait returns nil even when ctx ends, so the canceled context reaches
// CommitQuota's next query rather than this hook.
func (p *quotaPause) wait(ctx context.Context) error {
	if !p.started.CompareAndSwap(false, true) {
		return nil
	}
	close(p.paused)
	select {
	case <-p.resume:
	case <-ctx.Done():
	}
	return nil
}

// quotaHookStore runs hooks inside the quota transaction around the consumed
// quota read, counts quota attempt executions and lock tries, and injects
// errors.
type quotaHookStore struct {
	database.Store
	afterLookup    func(context.Context) error
	beforeTryLock  func(context.Context, database.Store) error
	beforeConsumed func(context.Context) error
	afterConsumed  func(context.Context) error
	onAttempt      func()
	afterAttempt   func(error)
	tryLocks       *atomic.Int32
	tryLockErr     error
	// attemptErr is joined to each quota attempt's result, like a COMMIT
	// that fails after the transaction ran.
	attemptErr error
}

func (s *quotaHookStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	attempt := opts.TxIdentifier == "commit_quota"
	err := s.Store.InTx(func(tx database.Store) error {
		if attempt && s.onAttempt != nil {
			s.onAttempt()
		}
		inner := *s
		inner.Store = tx
		err := fn(&inner)
		if attempt && s.afterAttempt != nil {
			s.afterAttempt(err)
		}
		return err
	}, opts)
	if attempt && s.attemptErr != nil {
		return errors.Join(err, s.attemptErr)
	}
	return err
}

func (s *quotaHookStore) GetWorkspaceByID(ctx context.Context, id uuid.UUID) (database.Workspace, error) {
	workspace, err := s.Store.GetWorkspaceByID(ctx, id)
	if err != nil {
		return database.Workspace{}, err
	}
	if s.afterLookup != nil {
		if err := s.afterLookup(ctx); err != nil {
			return database.Workspace{}, err
		}
	}
	return workspace, nil
}

func (s *quotaHookStore) TryAcquireLock(ctx context.Context, id int64) (bool, error) {
	if s.beforeTryLock != nil {
		if err := s.beforeTryLock(ctx, s.Store); err != nil {
			return false, err
		}
	}
	if s.tryLocks != nil {
		s.tryLocks.Add(1)
	}
	if s.tryLockErr != nil {
		return false, s.tryLockErr
	}
	return s.Store.TryAcquireLock(ctx, id)
}

func (s *quotaHookStore) GetQuotaConsumedForUser(ctx context.Context, arg database.GetQuotaConsumedForUserParams) (int64, error) {
	if s.beforeConsumed != nil {
		if err := s.beforeConsumed(ctx); err != nil {
			return 0, err
		}
	}
	consumed, err := s.Store.GetQuotaConsumedForUser(ctx, arg)
	if err != nil {
		return 0, err
	}
	if s.afterConsumed != nil {
		if err := s.afterConsumed(ctx); err != nil {
			return 0, err
		}
	}
	return consumed, nil
}

var errForcedRetry = xerrors.New("forced retry")

// retryOnceStore rolls back the first transaction attempt after its
// closure succeeds, then runs the closure again, the way InTx retries a
// SERIALIZABLE transaction whose commit reports a serialization failure.
type retryOnceStore struct {
	database.Store
	betweenAttempts func()
}

func (s *retryOnceStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	if opts.TxIdentifier != "commit_quota" {
		return s.Store.InTx(fn, opts)
	}
	err := s.Store.InTx(func(tx database.Store) error {
		if err := fn(tx); err != nil {
			return err
		}
		return errForcedRetry
	}, opts)
	if !errors.Is(err, errForcedRetry) {
		return xerrors.Errorf("first attempt: %w", err)
	}
	s.betweenAttempts()
	return s.Store.InTx(fn, opts)
}
