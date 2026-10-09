package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/semaphore"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// longLockTimeout keeps the lock timeout out of reach of driveBackoff, which
// advances the mock clock through every backoff.
const longLockTimeout = 24 * time.Hour

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
		admission := semaphore.NewWeighted(quotaAdmissionLimit(poolSize))
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

	// A slow commit keeps its admission slot, and other owners' commits on
	// the replica wait for it without using a connection.
	t.Run("AdmissionLimitAppliesAcrossOwners", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		otherUser := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 10, user, otherUser)

		pause := newQuotaPause()
		holder := f.committer(t, pause.store(f.db))
		holder.admission = semaphore.NewWeighted(1)
		holderDone := commitAsync(ctx, holder, f.build(t, user, org), 10)
		testutil.TryReceive(ctx, t, pause.paused)

		clock := quartz.NewMock(t)
		admit := clock.Trap().AfterFunc("commitQuota", "admission")
		defer admit.Close()
		otherDB := f.pool(t, 1)
		other := f.committer(t, database.New(otherDB))
		other.admission, other.clock = holder.admission, clock
		otherDone := commitAsync(ctx, other, f.build(t, otherUser, org), 10)
		admit.MustWait(ctx).MustRelease(ctx)
		clock.Advance(quotaLockTimeout).MustWait(ctx)
		require.ErrorContains(t, testutil.RequireReceive(ctx, t, otherDone).err, "timed out after 30s waiting for workspace quota lock")
		require.Zero(t, otherDB.Stats().OpenConnections)

		close(pause.resume)
		require.NoError(t, testutil.RequireReceive(ctx, t, holderDone).err)
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

	// Only a miss is retried. A retry here would wait on a mock clock that
	// nothing advances.
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
		held, err := quotaDB.Conn(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { _ = held.Close() })
		clock := quartz.NewMock(t)
		admit := clock.Trap().AfterFunc("commitQuota", "admission")
		defer admit.Close()
		c := f.committer(t, database.New(quotaDB))
		c.clock = clock

		done := commitAsync(ctx, c, build, 10)
		admit.MustWait(ctx).MustRelease(ctx)
		testutil.Eventually(ctx, t, func(context.Context) bool { return quotaDB.Stats().WaitCount == 1 }, testutil.IntervalFast)
		clock.Advance(quotaLockTimeout).MustWait(ctx)
		require.ErrorContains(t, testutil.RequireReceive(ctx, t, done).err, "timed out after 30s waiting for workspace quota lock")

		commitCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		done = commitAsync(commitCtx, c, build, 10)
		admit.MustWait(ctx).MustRelease(ctx)
		testutil.Eventually(ctx, t, func(context.Context) bool { return quotaDB.Stats().WaitCount == 2 }, testutil.IntervalFast)
		cancel()
		require.ErrorIs(t, testutil.RequireReceive(ctx, t, done).err, context.Canceled)
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

	t.Run("SameBuildDeadlineUpdateNeedsOneAttempt", func(t *testing.T) {
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
		require.EqualValues(t, 1, attempts.Load(), "serialization retries must not hide the conflict")
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
	return &committer{
		Log:       testutil.Logger(t),
		Database:  store,
		admission: semaphore.NewWeighted(quotaAdmissionLimit(0)),
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
	beforeConsumed func(context.Context) error
	afterConsumed  func(context.Context) error
	onAttempt      func()
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
		return fn(&inner)
	}, opts)
	if attempt && s.attemptErr != nil {
		return errors.Join(err, s.attemptErr)
	}
	return err
}

func (s *quotaHookStore) TryAcquireLock(ctx context.Context, id int64) (bool, error) {
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
