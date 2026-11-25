package coderd

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestCommitQuotaRetry(t *testing.T) {
	t.Parallel()

	serialErr := &pq.Error{Code: pq.ErrorCode("40001")}
	identityJitter := func(d time.Duration) time.Duration { return d }

	type result struct {
		resp *proto.CommitQuotaResponse
		err  error
	}

	setup := func(t *testing.T, faults *quotaFaults) (*committer, *quartz.Mock, *proto.CommitQuotaRequest) {
		t.Helper()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		user := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
		ws := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
		}).Do()
		clock := quartz.NewMock(t)
		c := &committer{
			Log:      testutil.Logger(t),
			Database: &quotaFaultStore{Store: db, faults: faults},
			Clock:    clock,
			jitter:   identityJitter,
		}
		return c, clock, &proto.CommitQuotaRequest{JobId: ws.Build.JobID.String(), DailyCost: 10}
	}

	commit := func(ctx context.Context, c *committer, req *proto.CommitQuotaRequest) <-chan result {
		ch := make(chan result, 1)
		go func() {
			resp, err := c.CommitQuota(ctx, req)
			ch <- result{resp: resp, err: err}
		}()
		return ch
	}

	t.Run("RetriesWithBackoff", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// More failures than the default of 3 attempts tolerates.
		const failures = 5
		faults := &quotaFaults{
			allowance: func(int) int64 { return 100 },
			consumedErr: func(attempt int) error {
				if attempt <= failures {
					return serialErr
				}
				return nil
			},
		}
		c, clock, req := setup(t, faults)
		trap := clock.Trap().NewTimer("CommitQuota", "backoff")
		defer trap.Close()

		resCh := commit(ctx, c, req)
		want := []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond}
		for _, d := range want {
			call := trap.MustWait(ctx)
			require.Equal(t, d, call.Duration)
			call.MustRelease(ctx)
			clock.Advance(d).MustWait(ctx)
		}

		res := testutil.RequireReceive(ctx, t, resCh)
		require.NoError(t, res.err)
		require.True(t, res.resp.Ok)
		require.EqualValues(t, 10, res.resp.CreditsConsumed)
		require.EqualValues(t, 100, res.resp.Budget)
		require.EqualValues(t, failures+1, faults.attempts.Load())
	})

	t.Run("ExhaustsRetries", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		faults := &quotaFaults{
			allowance:   func(int) int64 { return 100 },
			consumedErr: func(int) error { return serialErr },
		}
		c, clock, req := setup(t, faults)
		trap := clock.Trap().NewTimer("CommitQuota", "backoff")
		defer trap.Close()

		want := []time.Duration{
			50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond,
			1600 * time.Millisecond, 2 * time.Second, 2 * time.Second, 2 * time.Second, 2 * time.Second,
		}
		require.Len(t, want, commitQuotaMaxRetries)

		resCh := commit(ctx, c, req)
		for i := range want {
			call := trap.MustWait(ctx)
			require.Equal(t, want[i], call.Duration)
			call.MustRelease(ctx)
			clock.Advance(call.Duration).MustWait(ctx)
		}

		res := testutil.RequireReceive(ctx, t, resCh)
		require.True(t, database.IsSerializedError(res.err))
		require.EqualValues(t, commitQuotaMaxRetries+1, faults.attempts.Load())
	})

	t.Run("NonSerializationErrorNotRetried", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		boom := xerrors.New("boom")
		faults := &quotaFaults{
			allowance:   func(int) int64 { return 100 },
			consumedErr: func(int) error { return boom },
		}
		c, _, req := setup(t, faults)

		_, err := c.CommitQuota(ctx, req)
		require.ErrorIs(t, err, boom)
		require.EqualValues(t, 1, faults.attempts.Load())
	})

	t.Run("ContextCanceledDuringBackoff", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		faults := &quotaFaults{
			allowance:   func(int) int64 { return 100 },
			consumedErr: func(int) error { return serialErr },
		}
		c, clock, req := setup(t, faults)
		trap := clock.Trap().NewTimer("CommitQuota", "backoff")
		defer trap.Close()

		commitCtx, cancel := context.WithCancel(ctx)
		resCh := commit(commitCtx, c, req)
		call := trap.MustWait(ctx)
		cancel()
		call.MustRelease(ctx)

		res := testutil.RequireReceive(ctx, t, resCh)
		require.ErrorIs(t, res.err, context.Canceled)
		require.EqualValues(t, 1, faults.attempts.Load())
	})

	t.Run("RolledBackAttemptDoesNotLeak", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// The first attempt permits the build and then fails as if at
		// commit. The retry sees no budget and must reject.
		faults := &quotaFaults{
			allowance: func(attempt int) int64 {
				if attempt == 1 {
					return 100
				}
				return 0
			},
			afterBodyErr: func(attempt int) error {
				if attempt == 1 {
					return serialErr
				}
				return nil
			},
		}
		c, clock, req := setup(t, faults)
		trap := clock.Trap().NewTimer("CommitQuota", "backoff")
		defer trap.Close()

		resCh := commit(ctx, c, req)
		call := trap.MustWait(ctx)
		call.MustRelease(ctx)
		clock.Advance(call.Duration).MustWait(ctx)

		res := testutil.RequireReceive(ctx, t, resCh)
		require.NoError(t, res.err)
		require.False(t, res.resp.Ok)
		require.EqualValues(t, 0, res.resp.CreditsConsumed)
		require.EqualValues(t, 0, res.resp.Budget)
		require.EqualValues(t, 2, faults.attempts.Load())
	})
}

// quotaFaults injects failures into CommitQuota transactions. Each callback
// receives the 1-indexed transaction attempt.
type quotaFaults struct {
	attempts atomic.Int64
	// consumedErr, if it returns non-nil, fails GetQuotaConsumedForUser.
	consumedErr func(attempt int) error
	// allowance replaces GetQuotaAllowanceForUser.
	allowance func(attempt int) int64
	// afterBodyErr, if it returns non-nil, fails the transaction after the
	// body succeeds, simulating a failure at commit.
	afterBodyErr func(attempt int) error
}

type quotaFaultStore struct {
	database.Store
	faults  *quotaFaults
	attempt int
}

func (s *quotaFaultStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		attempt := int(s.faults.attempts.Add(1))
		if err := fn(&quotaFaultStore{Store: tx, faults: s.faults, attempt: attempt}); err != nil {
			return err
		}
		if s.faults.afterBodyErr != nil {
			return s.faults.afterBodyErr(attempt)
		}
		return nil
	}, opts)
}

func (s *quotaFaultStore) GetQuotaConsumedForUser(ctx context.Context, arg database.GetQuotaConsumedForUserParams) (int64, error) {
	if s.faults.consumedErr != nil {
		if err := s.faults.consumedErr(s.attempt); err != nil {
			return 0, err
		}
	}
	return s.Store.GetQuotaConsumedForUser(ctx, arg)
}

func (s *quotaFaultStore) GetQuotaAllowanceForUser(ctx context.Context, arg database.GetQuotaAllowanceForUserParams) (int64, error) {
	if s.faults.allowance != nil {
		return s.faults.allowance(s.attempt), nil
	}
	return s.Store.GetQuotaAllowanceForUser(ctx, arg)
}
