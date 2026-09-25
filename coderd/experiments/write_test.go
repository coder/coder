package experiments_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/experiments"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

var (
	ruleOn  = experiments.Rule{Mode: experiments.ModeOn}
	ruleOff = experiments.Rule{Mode: experiments.ModeOff}
	// ruleInherit is what reset writes.
	ruleInherit = experiments.Rule{Mode: experiments.ModeInherit}
)

// writeRule calls WriteRule and requires success.
func writeRule(ctx context.Context, t *testing.T, db database.Store, actor uuid.UUID, rule experiments.Rule, expected int64) (experiments.Rule, experiments.Rule, bool) {
	t.Helper()
	oldRule, newRule, changed, err := experiments.WriteRule(ctx, db, actor, scoped, rule, expected)
	require.NoError(t, err)
	return oldRule, newRule, changed
}

// storedValue returns the raw stored value for the scoped experiment.
func storedValue(ctx context.Context, t *testing.T, db database.Store) string {
	t.Helper()
	value, err := db.GetExperimentRule(ctx, string(scoped))
	require.NoError(t, err)
	return value
}

func requireConflict(t *testing.T, err error, expected, current int64) {
	t.Helper()
	var conflict *experiments.RevisionConflictError
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, expected, conflict.Expected)
	require.Equal(t, current, conflict.Current)
}

func TestWriteRule(t *testing.T) {
	t.Parallel()

	t.Run("RevisionsIncrease", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)
		actor := uuid.New()

		condition := experiments.Rule{Mode: experiments.ModeCondition, Condition: `user.username == "alice"`}
		previous := experiments.Rule{}
		for i, rule := range []experiments.Rule{ruleOn, ruleOff, condition, ruleInherit} {
			oldRule, newRule, changed := writeRule(ctx, t, db, actor, rule, int64(i))
			require.True(t, changed)
			require.Equal(t, previous, oldRule)
			require.Equal(t, rule.Mode, newRule.Mode)
			require.Equal(t, rule.Condition, newRule.Condition)
			require.Equal(t, int64(i+1), newRule.Revision)
			require.Equal(t, actor, newRule.UpdatedBy)
			require.False(t, newRule.UpdatedAt.IsZero())
			previous = newRule
		}
	})

	t.Run("IdenticalRuleIsNoOp", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)

		// The first inherit creates the row, so it is a change.
		oldRule, created, changed := writeRule(ctx, t, db, uuid.New(), ruleInherit, 0)
		require.True(t, changed)
		require.Equal(t, experiments.Rule{}, oldRule)
		require.Equal(t, int64(1), created.Revision)
		before := storedValue(ctx, t, db)

		// Another actor writing the same rule changes nothing.
		oldRule, newRule, changed := writeRule(ctx, t, db, uuid.New(), ruleInherit, 1)
		require.False(t, changed)
		require.Equal(t, created, oldRule)
		require.Equal(t, created, newRule)
		require.Equal(t, before, storedValue(ctx, t, db))
	})

	t.Run("StaleRevisionAfterResetAndRecreate", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)
		actor := uuid.New()

		writeRule(ctx, t, db, actor, ruleOn, 0)
		writeRule(ctx, t, db, actor, ruleInherit, 1) // reset
		writeRule(ctx, t, db, actor, ruleOn, 2)      // recreate
		before := storedValue(ctx, t, db)

		// A client that read revision 1 sees the same content again but
		// must still be rejected.
		_, _, _, err := experiments.WriteRule(ctx, db, actor, scoped, ruleOff, 1)
		requireConflict(t, err, 1, 3)
		_, _, _, err = experiments.WriteRule(ctx, db, actor, scoped, ruleOn, 0)
		requireConflict(t, err, 0, 3)
		require.Equal(t, before, storedValue(ctx, t, db))
	})

	t.Run("ConcurrentWritersOneWins", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
		actor := uuid.New()

		// Hold the write lock so that both writers are queued behind it
		// before either reads the current revision.
		holding := make(chan struct{})
		release := make(chan struct{})
		releaseLock := sync.OnceFunc(func() { close(release) })
		t.Cleanup(releaseLock)
		holderErr := make(chan error, 1)
		go func() {
			holderErr <- db.InTx(func(tx database.Store) error {
				if err := tx.AcquireLock(ctx, database.GenLockID("experiment_rule:"+string(scoped))); err != nil {
					return err
				}
				close(holding)
				<-release
				return nil
			}, nil)
		}()
		testutil.TryReceive(ctx, t, holding)

		results := make(chan error, 2)
		for range 2 {
			go func() {
				_, _, _, err := experiments.WriteRule(ctx, db, actor, scoped, ruleOn, 0)
				results <- err
			}()
		}
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			var waiting int
			err := sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM pg_locks
				WHERE locktype = 'advisory' AND NOT granted
				AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`).Scan(&waiting)
			return err == nil && waiting == 2
		}, testutil.IntervalFast, "both writers wait for the lock")
		releaseLock()
		require.NoError(t, testutil.TryReceive(ctx, t, holderErr))

		var wins int
		for range 2 {
			err := testutil.TryReceive(ctx, t, results)
			if err == nil {
				wins++
				continue
			}
			requireConflict(t, err, 0, 1)
		}
		require.Equal(t, 1, wins)

		var stored experiments.Rule
		require.NoError(t, json.Unmarshal([]byte(storedValue(ctx, t, db)), &stored))
		require.Equal(t, int64(1), stored.Revision)
	})

	t.Run("ErrorAfterWriteRollsBack", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)
		actor := uuid.New()

		writeRule(ctx, t, db, actor, ruleOn, 0)
		before := storedValue(ctx, t, db)

		_, _, _, err := experiments.WriteRule(ctx, failAfterUpsert{db}, actor, scoped, ruleOff, 1)
		require.ErrorIs(t, err, errInjected)
		require.Equal(t, before, storedValue(ctx, t, db))
	})

	t.Run("InvalidRuleNotStored", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)
		actor := uuid.New()

		_, _, _, err := experiments.WriteRule(ctx, db, actor, unscoped, ruleOn, 0)
		require.Error(t, err)
		_, _, _, err = experiments.WriteRule(ctx, db, actor, scoped, experiments.Rule{Mode: experiments.ModeCondition, Condition: "user."}, 0)
		require.Error(t, err)

		rows, err := db.GetExperimentRules(ctx)
		require.NoError(t, err)
		require.Empty(t, rows)
	})

	// The Evaluator reads what WriteRule stores, through NewDBStore.
	t.Run("EvaluatorReadsWrittenRule", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)
		alice := dbgen.User(t, db, database.User{Username: "alice"})
		bob := dbgen.User(t, db, database.User{Username: "bob"})
		evaluator, err := experiments.New(slogtest.Make(t, nil), experiments.NewDBStore(db), codersdk.Experiments{scoped})
		require.NoError(t, err)

		writeRule(ctx, t, db, alice.ID, experiments.Rule{Mode: experiments.ModeCondition, Condition: `user.username == "alice"`}, 0)
		require.True(t, evaluator.Enabled(ctx, alice.ID, scoped))
		require.False(t, evaluator.Enabled(ctx, bob.ID, scoped))

		// A kill switch applies to the next decision.
		writeRule(ctx, t, db, alice.ID, ruleOff, 1)
		require.False(t, evaluator.Enabled(ctx, alice.ID, scoped))
	})
}

var errInjected = xerrors.New("injected failure")

// failAfterUpsert fails every rule upsert after it runs on the transaction.
type failAfterUpsert struct {
	database.Store
}

func (s failAfterUpsert) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		return fn(failAfterUpsert{tx})
	}, opts)
}

func (s failAfterUpsert) UpsertExperimentRule(ctx context.Context, arg database.UpsertExperimentRuleParams) error {
	if err := s.Store.UpsertExperimentRule(ctx, arg); err != nil {
		return err
	}
	return errInjected
}
