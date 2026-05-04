package database_test

import (
	"database/sql"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
)

func TestReadModifyUpdate_OK(t *testing.T) {
	t.Parallel()

	mDB := dbmock.NewMockStore(gomock.NewController(t))

	mDB.EXPECT().InTransaction().Return(false)
	mDB.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		Times(1).
		Return(nil)
	err := database.ReadModifyUpdate(mDB, func(tx database.Store) error {
		return nil
	})
	require.NoError(t, err)
}

func TestReadModifyUpdate_RetryOK(t *testing.T) {
	t.Parallel()

	mDB := dbmock.NewMockStore(gomock.NewController(t))

	mDB.EXPECT().InTransaction().Return(false)
	firstUpdate := mDB.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		Times(1).
		Return(&pq.Error{Code: pq.ErrorCode("40001")})
	mDB.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		After(firstUpdate).
		Times(1).
		Return(nil)

	err := database.ReadModifyUpdate(mDB, func(tx database.Store) error {
		return nil
	})
	require.NoError(t, err)
}

func TestReadModifyUpdate_HardError(t *testing.T) {
	t.Parallel()

	mDB := dbmock.NewMockStore(gomock.NewController(t))

	mDB.EXPECT().InTransaction().Return(false)
	mDB.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		Times(1).
		Return(xerrors.New("a bad thing happened"))

	err := database.ReadModifyUpdate(mDB, func(tx database.Store) error {
		return nil
	})
	require.ErrorContains(t, err, "a bad thing happened")
}

func TestReadModifyUpdate_TooManyRetries(t *testing.T) {
	t.Parallel()

	mDB := dbmock.NewMockStore(gomock.NewController(t))

	mDB.EXPECT().InTransaction().Return(false)
	mDB.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		Times(5).
		Return(&pq.Error{Code: pq.ErrorCode("40001")})
	err := database.ReadModifyUpdate(mDB, func(tx database.Store) error {
		return nil
	})
	require.ErrorContains(t, err, "too many errors")
}

// TestReadModifyUpdate_NestedSkipsRetry verifies that a nested
// ReadModifyUpdate (one whose Store is already in a transaction) does not
// run its own retry loop. The inner closure must be invoked exactly once
// per outer attempt; on a serialization failure the error must propagate
// to the outermost ReadModifyUpdate, which retries.
func TestReadModifyUpdate_NestedSkipsRetry(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mOuter := dbmock.NewMockStore(ctrl)
	mInner := dbmock.NewMockStore(ctrl)

	// Outer is the top-level Store. RMU asks once and runs its retry loop.
	mOuter.EXPECT().InTransaction().Return(false)

	// Outer InTx is invoked twice: first attempt fails with 40001, second
	// attempt succeeds. DoAndReturn lets us run the user closure so the
	// nested RMU below executes against mInner.
	attempt := 0
	mOuter.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		Times(2).
		DoAndReturn(func(f func(database.Store) error, _ *database.TxOptions) error {
			attempt++
			err := f(mInner)
			if err != nil {
				return err
			}
			return nil
		})

	// Inner is the in-transaction Store. RMU asks once per outer attempt
	// and takes the fast path (no retry loop). The inner InTx is invoked
	// once per outer attempt: the first attempt returns 40001, the second
	// returns nil. Crucially, mInner.InTx is NOT called more than twice
	// total; proof that the inner RMU did not retry.
	mInner.EXPECT().InTransaction().Return(true).Times(2)
	firstInner := mInner.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		Times(1).
		Return(&pq.Error{Code: pq.ErrorCode("40001")})
	mInner.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		After(firstInner).
		Times(1).
		Return(nil)

	innerCalls := 0
	err := database.ReadModifyUpdate(mOuter, func(tx database.Store) error {
		innerCalls++
		return database.ReadModifyUpdate(tx, func(_ database.Store) error {
			return nil
		})
	})
	require.NoError(t, err)
	require.Equal(t, 2, attempt, "outer InTx should run twice")
	require.Equal(t, 2, innerCalls, "user closure should run twice")
}
