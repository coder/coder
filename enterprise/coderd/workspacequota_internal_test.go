package coderd

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestCommitQuotaConcurrency(t *testing.T) {
	t.Parallel()

	// Two cost-bearing builds for one owner and organization, with budget
	// for only one. The second commit must wait for the first, then see
	// its cost and deny.
	t.Run("SameOwnerAndOrgTakeTurns", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 10, user)
		first := f.build(t, user, org)
		second := f.build(t, user, org)

		pause := newQuotaPause(ctx)
		firstDone := commitAsync(ctx, f.committer(t, pause.store(f.db)), first, 10)
		testutil.TryReceive(ctx, t, pause.paused)

		secondDone := commitAsync(ctx, f.committer(t, f.db), second, 10)
		f.waitForQuotaLockWaiters(ctx, t, 1, secondDone)

		close(pause.resume)
		firstRes := testutil.RequireReceive(ctx, t, firstDone)
		require.NoError(t, firstRes.err)
		require.True(t, firstRes.resp.Ok)

		secondRes := testutil.RequireReceive(ctx, t, secondDone)
		require.NoError(t, secondRes.err)
		require.False(t, secondRes.resp.Ok, "second commit should see the first commit's cost")
		require.EqualValues(t, 10, secondRes.resp.CreditsConsumed)
		require.EqualValues(t, 10, secondRes.resp.Budget)

		require.EqualValues(t, 10, f.consumed(ctx, t, user, org))
	})

	// Many cost-bearing builds for one owner and organization at once,
	// with budget for two of them.
	t.Run("BurstRespectsBudget", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 20, user)

		const builds = 6
		results := make([]<-chan commitResult, 0, builds)
		for range builds {
			results = append(results, commitAsync(ctx, f.committer(t, f.db), f.build(t, user, org), 10))
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

		pause := newQuotaPause(ctx)
		heldDone := commitAsync(ctx, f.committer(t, pause.store(f.db)), f.build(t, user, org), 10)
		testutil.TryReceive(ctx, t, pause.paused)

		otherOwner := testutil.RequireReceive(ctx, t,
			commitAsync(ctx, f.committer(t, f.db), f.build(t, otherUser, org), 10))
		require.NoError(t, otherOwner.err)
		require.True(t, otherOwner.resp.Ok)

		otherOrgRes := testutil.RequireReceive(ctx, t,
			commitAsync(ctx, f.committer(t, f.db), f.build(t, user, otherOrg), 10))
		require.NoError(t, otherOrgRes.err)
		require.True(t, otherOrgRes.resp.Ok)

		close(pause.resume)
		held := testutil.RequireReceive(ctx, t, heldDone)
		require.NoError(t, held.err)
		require.True(t, held.resp.Ok)
	})

	// InTx can run the closure more than once. The response must come from
	// the attempt that committed, not from an earlier attempt that was
	// rolled back after permitting the build.
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
		require.Zero(t, f.quotaLocks(ctx, t))

		res, err := f.committer(t, f.db).CommitQuota(ctx, commitRequest(f.build(t, user, org), 10))
		require.NoError(t, err)
		require.True(t, res.Ok)
	})

	t.Run("CancelReleasesLock", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		f := newQuotaFixture(t)
		user := dbgen.User(t, f.db, database.User{})
		org := f.org(t, 10, user)

		holderCtx, cancelHolder := context.WithCancel(ctx)
		defer cancelHolder()
		pause := newQuotaPause(ctx)
		holderDone := commitAsync(holderCtx, f.committer(t, pause.store(f.db)), f.build(t, user, org), 10)
		testutil.TryReceive(ctx, t, pause.paused)

		// A commit canceled while it waits for the lock gives up.
		waiterCtx, cancelWaiter := context.WithCancel(ctx)
		defer cancelWaiter()
		waiterDone := commitAsync(waiterCtx, f.committer(t, f.db), f.build(t, user, org), 10)
		f.waitForQuotaLockWaiters(ctx, t, 1, waiterDone)
		cancelWaiter()
		require.Error(t, testutil.RequireReceive(ctx, t, waiterDone).err)

		// A commit canceled while it holds the lock rolls back and
		// releases it.
		cancelHolder()
		close(pause.resume)
		require.Error(t, testutil.RequireReceive(ctx, t, holderDone).err)
		require.Zero(t, f.quotaLocks(ctx, t))

		res, err := f.committer(t, f.db).CommitQuota(ctx, commitRequest(f.build(t, user, org), 10))
		require.NoError(t, err)
		require.True(t, res.Ok)
		require.EqualValues(t, 10, f.consumed(ctx, t, user, org))
	})
}

type quotaFixture struct {
	db    database.Store
	sqlDB *sql.DB
}

func newQuotaFixture(t *testing.T) quotaFixture {
	t.Helper()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	return quotaFixture{db: db, sqlDB: sqlDB}
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
	return &committer{Log: testutil.Logger(t), Database: store}
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

// quotaLocks counts advisory locks held or awaited in this test's database.
func (f quotaFixture) quotaLocks(ctx context.Context, t *testing.T) int {
	t.Helper()
	return f.countAdvisoryLocks(ctx, t, "")
}

func (f quotaFixture) countAdvisoryLocks(ctx context.Context, t *testing.T, extra string) int {
	t.Helper()
	var n int
	err := f.sqlDB.QueryRowContext(ctx, `
		SELECT count(*) FROM pg_locks
		WHERE locktype = 'advisory'
		AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`+extra,
	).Scan(&n)
	require.NoError(t, err)
	return n
}

// waitForQuotaLockWaiters waits until want commits are blocked on an
// advisory lock. It fails if done delivers first, which means the commit
// did not wait.
func (f quotaFixture) waitForQuotaLockWaiters(ctx context.Context, t *testing.T, want int, done <-chan commitResult) {
	t.Helper()
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		select {
		case res := <-done:
			require.FailNowf(t, "quota commit did not wait for the lock",
				"it finished while another commit for the same owner and organization was in progress: %+v", res)
		default:
		}
		return f.countAdvisoryLocks(ctx, t, " AND NOT granted") == want
	}, testutil.IntervalFast)
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

// quotaPause stops the first CommitQuota attempt just before it reads
// consumed quota, which is after the quota lock is acquired.
type quotaPause struct {
	testCtx context.Context
	once    sync.Once
	paused  chan struct{}
	resume  chan struct{}
}

func newQuotaPause(testCtx context.Context) *quotaPause {
	return &quotaPause{
		testCtx: testCtx,
		paused:  make(chan struct{}),
		resume:  make(chan struct{}),
	}
}

func (p *quotaPause) store(db database.Store) database.Store {
	return &quotaHookStore{Store: db, beforeConsumed: p.wait}
}

func (p *quotaPause) wait(context.Context) error {
	first := false
	p.once.Do(func() { first = true })
	if !first {
		return nil
	}
	close(p.paused)
	select {
	case <-p.resume:
	case <-p.testCtx.Done():
	}
	return nil
}

// quotaHookStore runs beforeConsumed inside the quota transaction, before
// CommitQuota reads consumed quota.
type quotaHookStore struct {
	database.Store
	beforeConsumed func(context.Context) error
}

func (s *quotaHookStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		return fn(&quotaHookStore{Store: tx, beforeConsumed: s.beforeConsumed})
	}, opts)
}

func (s *quotaHookStore) GetQuotaConsumedForUser(ctx context.Context, arg database.GetQuotaConsumedForUserParams) (int64, error) {
	if err := s.beforeConsumed(ctx); err != nil {
		return 0, err
	}
	return s.Store.GetQuotaConsumedForUser(ctx, arg)
}

var errForcedRetry = xerrors.New("forced retry")

// retryOnceStore rolls back the first transaction attempt after its
// closure succeeds, then runs the closure again, the way InTx retries a
// serialization failure reported at commit.
type retryOnceStore struct {
	database.Store
	betweenAttempts func()
}

func (s *retryOnceStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
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
